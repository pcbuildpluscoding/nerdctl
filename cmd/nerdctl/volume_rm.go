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
	"github.com/spf13/cobra"
)

func newVolumeRmCommand() *cobra.Command {
	volumeRmCommand := &cobra.Command{
		Use:               "rm [flags] VOLUME [VOLUME...]",
		Aliases:           []string{"remove"},
		Short:             "Remove one or more volumes",
		Long:              "NOTE: You cannot remove a volume that is in use by a container.",
		Args:              cobra.MinimumNArgs(1),
		RunE:              volumeRmAction,
		ValidArgsFunction: volumeRmShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	volumeRmCommand.Flags().BoolP("force", "f", false, "(unimplemented yet)")
	return volumeRmCommand
}

func volumeRmAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	return volume.Rm(cmd.Context(), &types.VolumeRmCommandOptions{
		GOptions: globalOptions,
		Volumes:  args,
	}, cmd.OutOrStdout())
}

func volumeRmShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show volume names
	return shellCompleteVolumeNames(cmd)
}
