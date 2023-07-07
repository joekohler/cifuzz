package cmdutils

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockCommand(name string) *cobra.Command {
	cmd := &cobra.Command{
		Use: name,
		PreRun: func(cmd *cobra.Command, args []string) {
			InitCurrentInvocation(cmd)
		},
		Run: func(cmd *cobra.Command, args []string) {},
	}

	return cmd
}

func TestSettingCommandNameWhenInvoked(t *testing.T) {
	name := "test"
	cmd := mockCommand(name)
	err := cmd.Execute()
	require.NoError(t, err)

	assert.Equal(t, name, CurrentInvocation.Command, "command name in current invocation is not set correctly")
}
