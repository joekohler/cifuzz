package init

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/log"
)

var testOut io.ReadWriter

func TestMain(m *testing.M) {
	// capture log output
	testOut = bytes.NewBuffer([]byte{})
	oldOut := log.Output
	log.Output = testOut
	viper.Set("verbose", true)

	m.Run()

	log.Output = oldOut
}

func TestInitCmd(t *testing.T) {
	testDir, cleanup := testutil.BootstrapExampleProjectForTest("init-cmd-test", config.BuildSystemCMake)
	defer cleanup()

	// remove cifuzz.yaml from example project
	err := os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)

	_, err = cmdutils.ExecuteCommand(t, New(), os.Stdin)
	assert.NoError(t, err)

	// second execution should return a ErrSilent as the config file should aready exists
	_, err = cmdutils.ExecuteCommand(t, New(), os.Stdin)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cmdutils.ErrSilent)
}

// TestInitCmdForNode tests the init command for Node.js projects (both JavaScript and TypeScript).
func TestInitCmdForNodeWithLanguageArg(t *testing.T) {
	if os.Getenv("CIFUZZ_PRERELEASE") == "" {
		t.Skip("skipping test for non-prerelease")
	}
	testDir, cleanup := testutil.BootstrapExampleProjectForTest("init-cmd-test", config.BuildSystemNodeJS)
	defer cleanup()

	// remove cifuzz.yaml from example project
	err := os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)

	// test for JavaScript
	_, err = cmdutils.ExecuteCommand(t, New(), os.Stdin, "js")
	assert.NoError(t, err)
	output, err := io.ReadAll(testOut)
	require.NoError(t, err)
	assert.Contains(t, string(output), "jest.config.js")
	assert.FileExists(t, filepath.Join(testDir, "cifuzz.yaml"))

	// remove cifuzz.yaml again
	err = os.Remove(filepath.Join(testDir, "cifuzz.yaml"))
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(testDir, "cifuzz.yaml"))

	// test for TypeScript
	_, err = cmdutils.ExecuteCommand(t, New(), os.Stdin, "ts")
	assert.NoError(t, err)
	output, err = io.ReadAll(testOut)
	require.NoError(t, err)
	assert.Contains(t, string(output), "jest.config.ts")
	assert.Contains(t, string(output), "To introduce the fuzz function types globally, add the following import to globals.d.ts:")
	assert.FileExists(t, filepath.Join(testDir, "cifuzz.yaml"))
}
