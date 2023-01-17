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
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/cmd/volume"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"

	"github.com/spf13/cobra"
)

func newVolumeLsCommand() *cobra.Command {
	volumeLsCommand := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List volumes",
		RunE:          volumeLsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	volumeLsCommand.Flags().BoolP("quiet", "q", false, "Only display volume names")
	// Alias "-f" is reserved for "--filter"
	volumeLsCommand.Flags().String("format", "", "Format the output using the given go template")
	volumeLsCommand.Flags().BoolP("size", "s", false, "Display the disk usage of volumes. Can be slow with volumes having loads of directories.")
	volumeLsCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	volumeLsCommand.Flags().StringSliceP("filter", "f", []string{}, "Filter matches volumes based on given conditions")
	return volumeLsCommand
}

func volumeLsAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	size, err := cmd.Flags().GetBool("size")
	if err != nil {
		return err
	}
	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return err
	}
	return volume.List(&types.VolumeListCommandOptions{
		GOptions: globalOptions,
		Quiet:    quiet,
		Format:   format,
		Size:     size,
		Filters:  filters,
	}, cmd.OutOrStdout())
}

func getVolumes(cmd *cobra.Command, globalOptions types.GlobalCommandOptions) (map[string]native.Volume, error) {
	volumeSize, err := cmd.Flags().GetBool("size")
	if err != nil {
		return nil, err
	}
	return volume.Volumes(globalOptions.Namespace, globalOptions.DataRoot, globalOptions.Address, volumeSize)
}
