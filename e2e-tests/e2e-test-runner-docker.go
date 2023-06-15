package e2e

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

var dockerClient *client.Client

// TODO: use the function from container command
// getDockerClient returns a docker client and will also handle its closing. It will take configuration options in the future.
func getDockerClient() (*client.Client, error) {
	if dockerClient != nil {
		return dockerClient, nil
	}
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer dockerClient.Close()
	return dockerClient, nil
}

func prepareDockerfile(t *testing.T) string {
	t.Helper()
	var baseImage string
	switch runtime.GOOS {
	case "linux", "darwin":
		baseImage = "ubuntu:latest"
	case "windows":
		baseImage = "mcr.microsoft.com/windows/server:ltsc2022"
	default:
		t.Fatal("unsupported OS")
	}
	dockerfile := "FROM " + baseImage + "\n"
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		dockerfile += "RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates\n"
	}
	return dockerfile
}

func buildImageFromDockerFile(t *testing.T, ctx context.Context, dockerClient *client.Client) {
	t.Helper()
	dockerfile := prepareDockerfile(t)
	dockerFolder := shared.CopyTestDockerDirForE2E(t, dockerfile)

	imageTar, err := container.CreateImageTar(dockerFolder)
	require.NoError(t, err)

	opts := types.ImageBuildOptions{
		Dockerfile:  "Dockerfile",
		Remove:      true,
		ForceRemove: true,
		Tags:        []string{"cifuzz-test:latest"},
	}

	res, err := dockerClient.ImageBuild(ctx, imageTar, opts)
	require.NoError(t, err)
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	t.Cleanup(func() { res.Body.Close() })
}

func runTestCaseInContainer(t *testing.T, ctx context.Context, dockerClient *client.Client, testCase *TestCase, testCaseRun testCaseRunOptions) CommandOutput {
	t.Helper()
	if testCase.CIUser != AnonymousCIUser {
		testCaseRun.args = "--server=" + ciServerToUseForE2ETests + " " + testCaseRun.args
	}

	cifuzzTargetMount := "/usr/bin/cifuzz"
	coverageDirectoryPath := "/coverage/e2e"
	if runtime.GOOS == "windows" {
		cifuzzTargetMount = "C:\\cifuzz"
		coverageDirectoryPath = "C:\\coverage\\e2e"
	}
	cifuzzExecutablePath := cifuzzTargetMount
	if runtime.GOOS == "windows" {
		cifuzzExecutablePath = cifuzzTargetMount + "\\cifuzz_windows.exe"
	}

	fmt.Println("Running test:", testCase.Description)
	fmt.Println("Command:", cifuzzExecutablePath, testCaseRun.command, testCaseRun.args)
	fmt.Println(" ")

	var hostCoverageDirectoryPath string
	hostCoverageDirectoryPath, _ = testutil.SetupCoverage(t, os.Environ(), "e2e")
	testCase.Environment = append(testCase.Environment, "GOCOVERDIR="+coverageDirectoryPath)

	targetMount := "/app"
	if runtime.GOOS == "windows" {
		targetMount = "C:\\app"
	}
	contextFolder := shared.CopyTestdataDirForE2E(t, testCaseRun.sampleFolder)
	t.Cleanup(func() {
		fileutil.Cleanup(contextFolder)
	})
	containerConfig := &dockerContainer.Config{
		Image:      "cifuzz-test:latest",
		Tty:        false,
		Env:        testCase.Environment,
		Cmd:        []string{cifuzzExecutablePath, testCaseRun.command},
		WorkingDir: targetMount,
	}

	if len(testCaseRun.args) > 0 {
		containerConfig.Cmd = append(containerConfig.Cmd, strings.Split(testCaseRun.args, " ")...)
	}

	containerConfig.Cmd = deleteEmptyStringsFromSlice(containerConfig.Cmd)

	cwd := testutil.RepoRoot(t)
	var cifuzzExecutableFile string
	switch runtime.GOOS {
	case "linux", "darwin":
		cifuzzExecutableFile = filepath.Join(cwd, "build", "bin", "cifuzz_linux")
	case "windows":
		cifuzzExecutableFile = filepath.Join(cwd, "build", "bin")
	default:
		t.Fatal("Unsupported OS")
	}

	cont, err := dockerClient.ContainerCreate(
		ctx,
		containerConfig,
		&dockerContainer.HostConfig{
			Binds: []string{
				cifuzzExecutableFile + ":" + cifuzzTargetMount,
				contextFolder + ":" + targetMount,
				hostCoverageDirectoryPath + ":" + coverageDirectoryPath,
			},
		},
		nil,
		nil,
		"", // TODO: should the container have a name?
	)
	require.NoError(t, err)

	err = dockerClient.ContainerStart(ctx, cont.ID, types.ContainerStartOptions{})
	require.NoError(t, err)

	var exitCode int64
	statusCh, errCh := dockerClient.ContainerWait(ctx, cont.ID, dockerContainer.WaitConditionNotRunning)
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	}

	out, err := dockerClient.ContainerLogs(ctx, cont.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
	require.NoError(t, err)

	containerStdOut := new(bytes.Buffer)
	containerStdErr := new(bytes.Buffer)
	_, err = stdcopy.StdCopy(containerStdOut, containerStdErr, out)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	stdOut := containerStdOut.String()
	stdErr := containerStdErr.String()

	return CommandOutput{
		ExitCode: int(exitCode),
		Stdout:   stdOut,
		Stderr:   stdErr,
		Stdall:   stdOut + stdErr,
		Workdir:  os.DirFS(contextFolder),
		t:        t,
	}
}

func deleteEmptyStringsFromSlice(s []string) []string {
	n := 0
	for _, str := range s {
		if str != "" {
			s[n] = str
			n++
		}
	}
	return s[:n]
}
