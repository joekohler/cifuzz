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
	buildDockerImage(t, imageTag, cifuzzRunner.DefaultWorkDir)

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

func buildDockerImage(t *testing.T, tag, dir string) {
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

	cmd = exec.Command("docker", "build", "-t", tag, dir)
	cmd.Env, err = envutil.Setenv(os.Environ(), "DOCKER_BUILDKIT", "1")
	require.NoError(t, err)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	t.Logf("Command: %s", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)
}
