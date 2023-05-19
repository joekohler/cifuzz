package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestOk(t *testing.T) {
	projectDir, err := os.Getwd()
	require.NoError(t, err)
	testDir := filepath.Join(projectDir, "buildlog_test")
	err = os.MkdirAll(testDir, 0755)
	require.NoError(t, err)
	defer fileutil.Cleanup(testDir)

	// no fuzz test
	expected := filepath.Join(testDir, ".cifuzz-build", "logs", "build-all.log")
	_, err = BuildOutputToFile(testDir, nil)
	require.NoError(t, err)
	assert.FileExists(t, expected)

	// one fuzz test
	fuzzTest := "my_fuzz_test"
	expected = filepath.Join(testDir, ".cifuzz-build", "logs", fmt.Sprintf("build-%s.log", fuzzTest))
	_, err = BuildOutputToFile(testDir, []string{fuzzTest})
	require.NoError(t, err)
	assert.FileExists(t, expected)

	// mutliple fuzz test
	fuzzTests := []string{"my_fuzz_test1", "my_fuzz_test2"}
	expected = filepath.Join(testDir, ".cifuzz-build", "logs", fmt.Sprintf("build-%s.log", strings.Join(fuzzTests, "_")))
	_, err = BuildOutputToFile(testDir, fuzzTests)
	require.NoError(t, err)
	assert.FileExists(t, expected)
}
