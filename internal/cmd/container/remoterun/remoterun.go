package remoterun

import (
	"fmt"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	return newWithOptions()
}

func newWithOptions() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote-run",
		Short: "Build and run a Fuzz Test container image on a CI server",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println("Called container remote-run!")
			return nil
		},
	}

	return cmd
}
