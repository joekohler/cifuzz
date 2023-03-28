package cmake

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/fileutil"
)

var baseTempDir string

func TestMain(m *testing.M) {
	var err error
	baseTempDir, err = os.MkdirTemp("", "cmake-test-")
	if err != nil {
		log.Fatalf("Failed to create temp dir for tests: %+v", err)
	}
	defer fileutil.Cleanup(baseTempDir)

	m.Run()
}

func TestNewBuilder(t *testing.T) {
	projectDir, err := os.MkdirTemp(baseTempDir, "project-dir-")
	require.NoError(t, err)

	// Create a builder
	builder1, err := NewBuilder(&BuilderOptions{
		ProjectDir: projectDir,
		Sanitizers: []string{"sanitizer1", "sanitizer2"},
		Stdout:     os.Stderr,
		Stderr:     os.Stderr,
	})
	require.NoError(t, err)
	buildDir1, err := builder1.BuildDir()
	require.NoError(t, err)
	require.DirExists(t, buildDir1)
	expectedBuildDir1 := filepath.Join(projectDir, ".cifuzz-build", "libfuzzer", "sanitizer1+sanitizer2")
	require.Equal(t, expectedBuildDir1, buildDir1)

	// Create another builder with additional args
	builder2, err := NewBuilder(&BuilderOptions{
		ProjectDir: projectDir,
		Args:       []string{"foo"},
		Sanitizers: []string{"sanitizer1", "sanitizer2"},
		Stdout:     os.Stderr,
		Stderr:     os.Stderr,
	})
	require.NoError(t, err)
	buildDir2, err := builder2.BuildDir()
	require.NoError(t, err)
	require.DirExists(t, buildDir2)
	// Check that the build dir name contains an additional hash value
	require.Equal(t, 8, len(strings.Split(filepath.Base(buildDir2), "-")[1]))

	// Check that the two builders have different build directories
	// (because they use different engines)
	require.NotEqual(t, buildDir1, buildDir2)

	// Create another builder without additional args
	builder3, err := NewBuilder(&BuilderOptions{
		ProjectDir: projectDir,
		Sanitizers: []string{"sanitizer1", "sanitizer2"},
		Stdout:     os.Stderr,
		Stderr:     os.Stderr,
	})
	require.NoError(t, err)
	buildDir3, err := builder3.BuildDir()
	require.NoError(t, err)
	require.DirExists(t, buildDir3)

	// Check that builder1 and builder3 have the same build directory
	// (because they use the same engine and sanitizers)
	require.Equal(t, buildDir1, buildDir3)
}
