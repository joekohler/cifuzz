package runfiles

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/fileutil"
)

var tempDirPath, tempDirCC string

func TestMain(m *testing.M) {
	tempDirPath = createDummyToolsInTempDir("test-path-")
	defer fileutil.Cleanup(tempDirPath)

	tempDirCC = createDummyToolsInTempDir("test-cc-")
	defer fileutil.Cleanup(tempDirCC)

	m.Run()
}

func TestIntegration_LLVMToolPath(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	finder := RunfilesFinderImpl{}

	// Check that llvm-cov is found in CC
	t.Setenv("CC", filepath.Join(tempDirCC, "clang"))
	t.Setenv("CXX", filepath.Join(tempDirCC, "clang++"))
	t.Setenv("PATH", tempDirPath)
	path, err := finder.llvmToolPath("llvm-cov")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDirCC, "llvm-cov"), path)

	// Check that llvm-cov is found in CXX if CC is not set
	t.Setenv("CC", "")
	path, err = finder.llvmToolPath("llvm-cov")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDirCC, "llvm-cov"), path)

	// Check that llvm-cov is found in PATH if CC and CXX are not set
	t.Setenv("CXX", "")
	path, err = finder.llvmToolPath("llvm-cov")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tempDirPath, "llvm-cov"), path)

	// Check that llvm-cov is not found if CC, CXX and PATH are not set
	t.Setenv("PATH", "")
	path, err = finder.llvmToolPath("llvm-cov")
	require.Error(t, err)
	assert.Equal(t, "", path)
}

func createDummyToolsInTempDir(dirName string) string {
	tempDir, err := os.MkdirTemp("", dirName)
	if err != nil {
		log.Fatalf("Failed to create temp dir for tests: %+v", err)
	}

	for _, tool := range []string{"clang", "clang++", "llvm-cov"} {
		file, err := os.OpenFile(filepath.Join(tempDir, tool), os.O_RDONLY|os.O_CREATE, 0755)
		if err != nil {
			log.Fatalf("Failed to create dummy file: %+v", err)
		}
		err = file.Close()
		if err != nil {
			log.Fatalf("Failed to create dummy %s for tests: %+v", tool, err)
		}
	}

	return tempDir
}
