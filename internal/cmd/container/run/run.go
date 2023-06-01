package run

import (
	"fmt"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	return newWithOptions()
}

func newWithOptions() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Build and run a Fuzz Test container image locally",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println("Called container run!")
			return nil
		},
	}

	return cmd
}
