package init

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
)

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
