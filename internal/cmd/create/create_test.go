package create

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/dependencies"
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

func TestOk(t *testing.T) {
	testDir, cleanup := testutil.BootstrapExampleProjectForTest("create-cmd-test", config.BuildSystemCMake)
	defer cleanup()

	outputFile := filepath.Join(testDir, "fuzz-test.cpp")
	args := []string{
		"cpp",
		"--output", outputFile,
	}
	_, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestOkMaven(t *testing.T) {
	testDir, cleanup := testutil.BootstrapExampleProjectForTest("create-cmd-test", config.BuildSystemMaven)
	defer cleanup()

	outputFile := filepath.Join(testDir, "FuzzTestCase.java")
	args := []string{
		"java",
		"--output", outputFile,
	}
	_, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestOkGradle(t *testing.T) {
	testDir, cleanup := testutil.BootstrapExampleProjectForTest("create-cmd-test", config.BuildSystemGradle)
	defer cleanup()

	outputFile := filepath.Join(testDir, "FuzzTestCase.java")
	args := []string{
		"java",
		"--output", outputFile,
	}
	_, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestOkJavaScript(t *testing.T) {
	if os.Getenv("CIFUZZ_PRERELEASE") == "" {
		t.Skip("Skipping test because CIFUZZ_PRERELEASE is not set.")
	}

	testDir, cleanup := testutil.BootstrapExampleProjectForTest("create-cmd-test", config.BuildSystemNodeJS)
	defer cleanup()

	outputFile := filepath.Join(testDir, "myTest.fuzz.js")
	args := []string{
		"js",
		"--output", outputFile,
	}
	_, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.NoError(t, err)
	require.FileExists(t, outputFile)
}

func TestInvalidType(t *testing.T) {
	args := []string{
		"foo",
	}
	_, err := cmdutils.ExecuteCommand(t, New(), os.Stdin, args...)
	require.Error(t, err)
}

func TestCreateCmd_OutDir(t *testing.T) {
	t.Skip()
}

func TestCMakeMissing(t *testing.T) {
	dependencies.TestMockAllDeps(t)
	dependencies.OverwriteUninstalled(dependencies.GetDep(dependencies.CMake))

	testDir, cleanup := testutil.BootstrapExampleProjectForTest("create-cmd-test", config.BuildSystemCMake)
	defer cleanup()
	args := []string{
		"cpp",
		"--output",
		filepath.Join(testDir, "fuzz-test.cpp"),
	}

	opts := &createOpts{
		BuildSystem: config.BuildSystemCMake,
	}

	_, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, args...)
	// should not fail as this command has no hard dependencies, just recommendations
	require.NoError(t, err)

	output, err := io.ReadAll(testOut)
	require.NoError(t, err)
	assert.Contains(t, string(output), fmt.Sprintf(dependencies.MessageMissing, "cmake"))
}

func TestClangVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	dependencies.TestMockAllDeps(t)
	dep := dependencies.GetDep(dependencies.Clang)
	version := dependencies.OverwriteGetVersionWith0(dep)

	testDir, cleanup := testutil.BootstrapExampleProjectForTest("create-cmd-test", config.BuildSystemCMake)
	defer cleanup()
	args := []string{
		"cpp",
		"--output",
		filepath.Join(testDir, "fuzz-test.cpp"),
	}

	opts := &createOpts{
		BuildSystem: config.BuildSystemCMake,
	}

	_, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, args...)
	// should not fail as this command has no hard dependencies, just recommendations
	require.NoError(t, err)

	output, err := io.ReadAll(testOut)
	require.NoError(t, err)
	assert.Contains(t, string(output),
		fmt.Sprintf(dependencies.MessageVersion, "clang", dep.MinVersion.String(), version))
}

func TestVisualStudioVersion(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}
	dependencies.TestMockAllDeps(t)
	dep := dependencies.GetDep(dependencies.VisualStudio)
	version := dependencies.OverwriteGetVersionWith0(dep)

	testDir, cleanup := testutil.BootstrapExampleProjectForTest("create-cmd-test", config.BuildSystemCMake)
	defer cleanup()
	args := []string{
		"cpp",
		"--output",
		filepath.Join(testDir, "fuzz-test.cpp"),
	}

	opts := &createOpts{
		BuildSystem: config.BuildSystemCMake,
	}

	_, err := cmdutils.ExecuteCommand(t, newWithOptions(opts), os.Stdin, args...)
	// should not fail as this command has no hard dependencies, just recommendations
	require.NoError(t, err)

	output, err := io.ReadAll(testOut)
	require.NoError(t, err)
	assert.Contains(t, string(output),
		fmt.Sprintf(dependencies.MessageVersion, "Visual Studio", dep.MinVersion.String(), version))
}
