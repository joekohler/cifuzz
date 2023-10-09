package shared

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/envutil"
)

func TestContainerRun(t *testing.T, cifuzzRunner *CIFuzzRunner, imageTag string, options *RunOptions) {
	// Build the cifuzz base image which is used by the container run
	// command to ensure that the base image contains the latest version
	// of the cifuzz binary.
	buildCIFuzzBaseImage(t)

	// Create a temporary directory which we mount into the container to
	// be able to access the generated corpus files and the coverage
	// report.
	outputDir := testutil.MkdirTemp(t, "", "cifuzz-container-run-output-*")

	options.Command = []string{"container", "run"}
	options.Args = append(options.Args,
		"--docker-image", imageTag,
		// Mount the output directory into the container
		"--bind", fmt.Sprintf("%s:/output", outputDir),
		// All other arguments are passed to the fuzz container. This
		// requires two "--" because arguments after the first "--" are
		// used as build system arguments and arguments after the second
		// "--" are used as container arguments.
		"--", "--",
		// Specify the generated corpus dir
		"--generated-corpus-dir", "/output/generated-corpus",
		// Produce an LCOV coverage report
		"--coverage-output-path", "/output/coverage.lcov",
	)

	cifuzzRunner.Run(t, options)

	// Check that files were created in the corpus directory
	entries, err := os.ReadDir(filepath.Join(outputDir, "generated-corpus"))
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	// Check that the corpus directory only contains files and no directories
	for _, entry := range entries {
		require.False(t, entry.IsDir())
	}

	// Check that the LCOV coverage report was created
	_, err = os.Stat(filepath.Join(outputDir, "coverage.lcov"))
	require.NoError(t, err)
}

func BuildDockerImage(t *testing.T, tag string, buildDir string) {
	var err error
	cmd := exec.Command("docker", "build", "-t", tag, buildDir)
	cmd.Env, err = envutil.Setenv(os.Environ(), "DOCKER_BUILDKIT", "1")
	require.NoError(t, err)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	t.Logf("Command: %s", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)
}

// This is not a test but a helper function so gocritic should not
// complain that it's not of the form TestXXX(t *testing.T).
//
//nolint:gocritic
func buildCIFuzzBaseImage(t *testing.T) {
	var err error

	cwd, err := os.Getwd()
	require.NoError(t, err)

	cmd := exec.Command("make", "build-container-image")
	cmd.Dir = filepath.Join(cwd, "..", "..")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	t.Logf("Command: %s", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)
}
