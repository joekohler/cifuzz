package reload

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/dependencies"
)

func TestMain(m *testing.M) {
	viper.Set("verbose", true)
	m.Run()
}

func TestReloadCmd_FailsIfNoCIFuzzProject(t *testing.T) {
	// Create an empty directory
	projectDir := testutil.MkdirTemp(t, "", "test-reload-cmd-fails-")

	opts := &options{
		ProjectDir: projectDir,
		ConfigDir:  projectDir,
	}

	// Check that the command produces the expected error when not
	// called below a cifuzz project directory.
	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)
	assert.Contains(t, stdErr, "Failed to parse cifuzz.yaml")
}

func TestClangMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	projectDir := testutil.BootstrapEmptyProject(t, "test-reload-")
	opts := &options{
		ProjectDir:  projectDir,
		ConfigDir:   projectDir,
		BuildSystem: config.BuildSystemCMake,
	}

	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.Clang))

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)
	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "clang"))
}

func TestVisualStudioMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}
	projectDir := testutil.BootstrapEmptyProject(t, "test-reload-")
	opts := &options{
		ProjectDir:  projectDir,
		ConfigDir:   projectDir,
		BuildSystem: config.BuildSystemCMake,
	}

	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.VisualStudio))

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)
	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "Visual Studio"))
}

func TestCMakeMissing(t *testing.T) {
	projectDir := testutil.BootstrapEmptyProject(t, "test-reload-")
	opts := &options{
		ProjectDir:  projectDir,
		ConfigDir:   projectDir,
		BuildSystem: config.BuildSystemCMake,
	}

	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.CMake))

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)
	assert.Contains(t, stdErr, fmt.Sprintf(dependencies.MessageMissing, "cmake"))
}

func TestWrongCMakeVersion(t *testing.T) {
	projectDir := testutil.BootstrapEmptyProject(t, "test-reload-")
	opts := &options{
		ProjectDir:  projectDir,
		ConfigDir:   projectDir,
		BuildSystem: config.BuildSystemCMake,
	}

	dependencies.TestMockAllDeps(t)
	dep := dependencies.GetDep(dependencies.CMake)
	version := dependencies.OverwriteGetVersionWith0(dep)

	_, stdErr, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin)
	require.Error(t, err)

	assert.Contains(t, stdErr,
		fmt.Sprintf(dependencies.MessageVersion, "cmake", dep.MinVersion.String(), version))
}
