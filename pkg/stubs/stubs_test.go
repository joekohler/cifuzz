package stubs

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hectane/go-acl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

var baseTempDir string

func TestMain(m *testing.M) {
	var err error
	baseTempDir, err = os.MkdirTemp("", "stubs-test-")
	if err != nil {
		log.Fatalf("Failed to create temp dir for tests: %+v", err)
	}
	defer fileutil.Cleanup(baseTempDir)
	m.Run()
}

func TestCreate(t *testing.T) {
	projectDir := testutil.MkdirTemp(t, baseTempDir, "project-")

	// Test .cpp files
	stubFile := filepath.Join(projectDir, "fuzz_test.cpp")
	err := Create(stubFile, config.CPP)
	assert.NoError(t, err)

	exists, err := fileutil.Exists(stubFile)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test .java files
	stubFile = filepath.Join(projectDir, "FuzzTestCase.java")
	err = Create(stubFile, config.Java)
	assert.NoError(t, err)

	exists, err = fileutil.Exists(stubFile)
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestCreate_Exists(t *testing.T) {
	projectDir := testutil.MkdirTemp(t, baseTempDir, "project-")

	// Test .cpp files
	stubFile := filepath.Join(projectDir, "fuzz_test.cpp")
	err := os.WriteFile(stubFile, []byte("TEST"), 0644)
	assert.NoError(t, err)

	err = Create(stubFile, config.CPP)
	assert.Error(t, err)
	assert.ErrorIs(t, err, os.ErrExist)

	// Test .java files
	stubFile = filepath.Join(projectDir, "FuzzTestCase.java")
	err = os.WriteFile(stubFile, []byte("TEST"), 0644)
	assert.NoError(t, err)

	err = Create(stubFile, config.Java)
	assert.Error(t, err)
	assert.ErrorIs(t, err, os.ErrExist)
}

func TestCreate_NoPerm(t *testing.T) {
	// create read only project dir
	projectDir := testutil.MkdirTemp(t, baseTempDir, "project-")
	err := acl.Chmod(projectDir, 0555)
	require.NoError(t, err)

	// Test .cpp files
	stubFile := filepath.Join(projectDir, "fuzz_test.cpp")
	err = Create(stubFile, config.CPP)
	assert.Error(t, err)
	assert.ErrorIs(t, err, os.ErrPermission)

	// Test .java files
	stubFile = filepath.Join(projectDir, "MyFuzzTest.java")
	err = Create(stubFile, config.Java)
	assert.Error(t, err)
	assert.ErrorIs(t, err, os.ErrPermission)
}

func TestSuggestFilename(t *testing.T) {
	projectDir := testutil.MkdirTemp(t, baseTempDir, "project-")
	err := os.Chdir(projectDir)
	require.NoError(t, err)

	// Test .cpp files
	filename1, err := FuzzTestFilename(config.CPP)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(".", "my_fuzz_test_1.cpp"), filename1)

	err = os.WriteFile(filename1, []byte("TEST"), 0644)
	require.NoError(t, err)

	filename2, err := FuzzTestFilename(config.CPP)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(".", "my_fuzz_test_2.cpp"), filename2)

	// Test .java files
	filename3, err := FuzzTestFilename(config.Java)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(".", "MyClassFuzzTest1.java"), filename3)

	err = os.WriteFile(filename3, []byte("TEST"), 0644)
	require.NoError(t, err)

	filename4, err := FuzzTestFilename(config.Java)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(".", "MyClassFuzzTest2.java"), filename4)
}

func TestCreateJavaFileAndClassName(t *testing.T) {
	projectDir := testutil.MkdirTemp(t, baseTempDir, "project-")
	err := os.Chdir(projectDir)
	require.NoError(t, err)

	// Test .java files
	stubName := "MyOwnPersonalFuzzTest.java"
	stubFile := filepath.Join(projectDir, stubName)
	err = Create(stubFile, config.Java)
	assert.NoError(t, err)

	exists, err := fileutil.Exists(stubFile)
	assert.NoError(t, err)
	assert.True(t, exists)

	testFile, err := os.ReadFile(stubFile)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(testFile), "class "+strings.TrimSuffix(stubName, ".java")))
}
