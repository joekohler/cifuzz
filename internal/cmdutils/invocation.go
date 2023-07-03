package cmdutils

import (
	"github.com/spf13/cobra"
)

type Invocation struct {
	Command string
}

var CurrentInvocation *Invocation

func InitCurrentInvocation(cmd *cobra.Command) {
	CurrentInvocation = &Invocation{Command: cmd.CalledAs()}
}
