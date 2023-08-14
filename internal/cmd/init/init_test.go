package init

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
)

func TestMain(m *testing.M) {
	viper.Set("verbose", true)
	m.Run()
}

func TestInitCmd(t *testing.T) {
	testDir, cleanup := testutil.BootstrapExampleProjectForTest("init-cmd-test", config.BuildSystemCMake)
	defer cleanup()

	// remove cifuzz.yaml from example project
	err := os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)

	_, _, err = cmdutils.ExecuteCommand(t, New(), os.Stdin)
	assert.NoError(t, err)

	// second execution should return a ErrSilent as the config file should aready exists
	_, _, err = cmdutils.ExecuteCommand(t, New(), os.Stdin)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cmdutils.ErrSilent)
}

// TestInitCmdForNode tests the init command for Node.js projects (both JavaScript and TypeScript).
func TestInitCmdForNodeWithLanguageArg(t *testing.T) {
	testDir, cleanup := testutil.BootstrapExampleProjectForTest("init-cmd-test", config.BuildSystemNodeJS)
	defer cleanup()

	// remove cifuzz.yaml from example project
	err := os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)

	// test for JavaScript
	_, stdErr, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, "js")
	assert.NoError(t, err)
	assert.Contains(t, stdErr, "jest.config.js")
	assert.FileExists(t, filepath.Join(testDir, "cifuzz.yaml"))

	// remove cifuzz.yaml again
	err = os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(testDir, "cifuzz.yaml"))

	// test for TypeScript
	_, stdErr, err = cmdutils.ExecuteCommand(t, New(), os.Stdin, "ts")
	assert.NoError(t, err)
	assert.Contains(t, stdErr, "jest.config.ts")
	assert.Contains(t, stdErr, "To introduce the fuzz function types globally, add the following import to globals.d.ts:")
	assert.FileExists(t, filepath.Join(testDir, "cifuzz.yaml"))
}
