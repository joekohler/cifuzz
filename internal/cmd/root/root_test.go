package root

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestMain(m *testing.M) {
	_, cleanup := testutil.ChdirToTempDir("init-cmd-test-")
	defer cleanup()

	m.Run()
}

func TestRootCmd(t *testing.T) {
	cmd, err := New()
	require.NoError(t, err)
	_, err = cmdutils.ExecuteCommand(t, cmd, os.Stdin)
	assert.NoError(t, err)
}

func TestChangingToNonExistingDirectory(t *testing.T) {
	origWorkDir, err := os.Getwd()
	require.NoError(t, err)

	args := []string{
		"-C", "foo",
		// The PersistentPreRunE function in which we change the
		// directory is only executed if a subcommand is specified,
		// else only the usage message is printed, so we specify a
		// subcommand.
		"init",
	}
	cmd, err := New()
	require.NoError(t, err)
	_, err = cmdutils.ExecuteCommand(t, cmd, os.Stdin, args...)
	require.Error(t, err)

	// Check that the working directory did not change
	workDir, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, origWorkDir, workDir)
}

func TestChangingToExistingDirectory(t *testing.T) {
	origWorkDir, err := os.Getwd()
	require.NoError(t, err)

	err = os.Mkdir("foo", 0700)
	require.NoError(t, err)
	err = fileutil.Touch(filepath.Join("foo", "CMakeLists.txt"))
	require.NoError(t, err)

	args := []string{
		"-C", "./foo",
		// The PersistentPreRunE function in which we change the
		// directory is only executed if a subcommand is specified,
		// else only the usage message is printed, so we specify a
		// subcommand.
		"init",
	}
	cmd, err := New()
	require.NoError(t, err)
	_, err = cmdutils.ExecuteCommand(t, cmd, os.Stdin, args...)
	require.NoError(t, err)

	// Check that the working directory actually changed
	workDir, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(origWorkDir, "foo"), workDir)
}
