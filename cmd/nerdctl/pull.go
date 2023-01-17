/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cosignutil"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/ipfs"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

func newPullCommand() *cobra.Command {
	var pullCommand = &cobra.Command{
		Use:           "pull [flags] NAME[:TAG]",
		Short:         "Pull an image from a registry. Optionally specify \"ipfs://\" or \"ipns://\" scheme to pull image from IPFS.",
		Args:          IsExactArgs(1),
		RunE:          pullAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pullCommand.Flags().String("unpack", "auto", "Unpack the image for the current single platform (auto/true/false)")
	pullCommand.RegisterFlagCompletionFunc("unpack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"auto", "true", "false"}, cobra.ShellCompDirectiveNoFileComp
	})

	// #region platform flags
	// platform is defined as StringSlice, not StringArray, to allow specifying "--platform=amd64,arm64"
	pullCommand.Flags().StringSlice("platform", nil, "Pull content for a specific platform")
	pullCommand.RegisterFlagCompletionFunc("platform", shellCompletePlatforms)
	pullCommand.Flags().Bool("all-platforms", false, "Pull content for all platforms")
	// #endregion

	// #region verify flags
	pullCommand.Flags().String("verify", "none", "Verify the image (none|cosign)")
	pullCommand.RegisterFlagCompletionFunc("verify", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"none", "cosign"}, cobra.ShellCompDirectiveNoFileComp
	})
	pullCommand.Flags().String("cosign-key", "", "Path to the public key file, KMS, URI or Kubernetes Secret for --verify=cosign")
	// #endregion

	pullCommand.Flags().BoolP("quiet", "q", false, "Suppress verbose output")

	pullCommand.Flags().String("ipfs-address", "", "multiaddr of IPFS API (default uses $IPFS_PATH env variable if defined or local directory ~/.ipfs)")

	return pullCommand
}

func pullAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	rawRef := args[0]
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	allPlatforms, err := cmd.Flags().GetBool("all-platforms")
	if err != nil {
		return err
	}
	platform, err := cmd.Flags().GetStringSlice("platform")
	if err != nil {
		return err
	}
	ocispecPlatforms, err := platformutil.NewOCISpecPlatformSlice(allPlatforms, platform)
	if err != nil {
		return err
	}

	unpackStr, err := cmd.Flags().GetString("unpack")
	if err != nil {
		return err
	}
	unpack, err := strutil.ParseBoolOrAuto(unpackStr)
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}

	_, err = ensureImage(ctx, cmd, client, globalOptions, rawRef, ocispecPlatforms, "always", unpack, quiet)
	if err != nil {
		return err
	}

	return nil
}

func ensureImage(ctx context.Context, cmd *cobra.Command, client *containerd.Client, globalOptions types.GlobalCommandOptions, rawRef string, ocispecPlatforms []v1.Platform, pull string, unpack *bool, quiet bool) (*imgutil.EnsuredImage, error) {

	var ensured *imgutil.EnsuredImage

	verifier, err := cmd.Flags().GetString("verify")
	if err != nil {
		return nil, err
	}

	if scheme, ref, err := referenceutil.ParseIPFSRefWithScheme(rawRef); err == nil {
		if verifier != "none" {
			return nil, errors.New("--verify flag is not supported on IPFS as of now")
		}

		ipfsAddressStr, err := cmd.Flags().GetString("ipfs-address")
		if err != nil {
			return nil, err
		}

		var ipfsPath *string
		if ipfsAddressStr != "" {
			dir, err := os.MkdirTemp("", "apidirtmp")
			if err != nil {
				return nil, err
			}
			defer os.RemoveAll(dir)
			if err := os.WriteFile(filepath.Join(dir, "api"), []byte(ipfsAddressStr), 0600); err != nil {
				return nil, err
			}
			ipfsPath = &dir
		}

		ensured, err = ipfs.EnsureImage(ctx, client, cmd.OutOrStdout(), cmd.ErrOrStderr(), globalOptions.Snapshotter, scheme, ref,
			pull, ocispecPlatforms, unpack, quiet, ipfsPath)
		if err != nil {
			return nil, err
		}
		return ensured, nil
	}

	ref := rawRef
	switch verifier {
	case "cosign":
		experimental := globalOptions.Experimental

		if !experimental {
			return nil, fmt.Errorf("cosign only work with enable experimental feature")
		}

		keyRef, err := cmd.Flags().GetString("cosign-key")
		if err != nil {
			return nil, err
		}

		ref, err = cosignutil.VerifyCosign(ctx, rawRef, keyRef, globalOptions.HostsDir)
		if err != nil {
			return nil, err
		}
	case "none":
		logrus.Debugf("verification process skipped")
	default:
		return nil, fmt.Errorf("no verifier found: %s", verifier)
	}

	ensured, err = imgutil.EnsureImage(ctx, client, cmd.OutOrStdout(), cmd.ErrOrStderr(), globalOptions.Snapshotter, ref,
		pull, globalOptions.InsecureRegistry, globalOptions.HostsDir, ocispecPlatforms, unpack, quiet)
	if err != nil {
		return nil, err
	}
	return ensured, err
}
