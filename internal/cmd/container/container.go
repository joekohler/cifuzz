package container

import (
	"github.com/spf13/cobra"

	containerRemoteRunCmd "code-intelligence.com/cifuzz/internal/cmd/container/remoterun"
	containerRunCmd "code-intelligence.com/cifuzz/internal/cmd/container/run"
)

func New() *cobra.Command {
	return newWithOptions()
}

func newWithOptions() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "container",
		Short: "Preview: better living through chemistry",
		Long:  `Preview of new cifuzz container capabilities. Aim is to improve local and remote Fuzz Test runs. Available only with CIFUZZ_PRERELEASE flag.`,
		RunE: func(c *cobra.Command, args []string) error {
			_ = c.Help()
			return nil
		},
	}

	cmd.AddCommand(containerRunCmd.New())
	cmd.AddCommand(containerRemoteRunCmd.New())

	return cmd
}
