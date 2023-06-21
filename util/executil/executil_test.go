package executil

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestCmd_SendSigterm(t *testing.T) {
	// Sending SIGTERM via cmd.Process.Signal is not supported on Windows
	if runtime.GOOS == "windows" {
		return
	}

	// Start a process which prints output and never exits on its own
	path := buildYes(t)
	cmd := Command(path)
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	require.NoError(t, err)

	go func() {
		time.Sleep(time.Second)

		// Terminate the process
		err = cmd.Process.Signal(syscall.SIGTERM)
		require.NoError(t, err)
	}()

	err = cmd.Wait()
	require.True(t, IsTerminatedExitErr(err))
}

func TestCmd_TerminateProcessGroup(t *testing.T) {
	// Start a process which prints output and never exits on its own
	path := buildYes(t)
	cmd := Command(path)
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	require.NoError(t, err)

	go func() {
		time.Sleep(time.Second)

		// Terminate the process
		err = cmd.TerminateProcessGroup()
		require.NoError(t, err)
	}()

	err = cmd.Wait()
	require.True(t, IsTerminatedExitErr(err))
}

func TestCmd_TerminateProcessGroup_With_StdoutTeePipe(t *testing.T) {
	// Start a process which prints output and never exits on its own
	path := buildYes(t)
	cmd := Command(path)
	cmd.Stderr = os.Stderr

	// Create a tee pipe to test the behavior of TerminateProcessGroup
	// when the command uses a tee pipe
	pipe, err := cmd.StdoutTeePipe(io.Discard)
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	go func() {
		// Read a line from the pipe
		scanner := bufio.NewScanner(pipe)
		scanner.Scan()
		require.NoError(t, scanner.Err())
		require.Equal(t, "y", scanner.Text())

		// Terminate the process
		err = cmd.TerminateProcessGroup()
		require.NoError(t, err)
	}()

	err = cmd.Wait()
	require.True(t, IsTerminatedExitErr(err))

	err = pipe.Close()
	require.NoError(t, err)
}

func TestCmd_StdoutTeePipe_ReadAsync(t *testing.T) {
	// Pipe stdout to a file
	outFile, err := os.CreateTemp("", "outFile")
	require.NoError(t, err)
	defer fileutil.Cleanup(outFile.Name())

	cmd := echoCommand("foo")
	pipe, err := cmd.StdoutTeePipe(outFile)
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	doneCh := make(chan struct{})
	go func() {
		outFromPipe, err := io.ReadAll(pipe)
		require.NoError(t, err)
		require.Equal(t, "foo", strings.TrimSpace(string(outFromPipe)))
		doneCh <- struct{}{}
	}()

	err = cmd.Wait()
	require.NoError(t, err)

	err = outFile.Close()
	require.NoError(t, err)

	outFromFile, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Equal(t, "foo", strings.TrimSpace(string(outFromFile)))

	// Wait until the pipe was read
	<-doneCh

	err = pipe.Close()
	require.NoError(t, err)
}

func TestCmd_StdoutTeePipe_ReadAsyncMultiline(t *testing.T) {
	cmd := Command("yes")
	pipe, err := cmd.StdoutTeePipe(io.Discard)
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	doneCh := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(pipe)
		scanner.Scan()
		require.Equal(t, "y", scanner.Text())
		doneCh <- struct{}{}
	}()

	// Wait until the pipe was read
	<-doneCh

	err = pipe.Close()
	require.NoError(t, err)
}

func TestCmd_StdoutTeePipe_ReadSync(t *testing.T) {
	// Pipe stdout to a file
	outFile, err := os.CreateTemp("", "outFile")
	require.NoError(t, err)
	defer fileutil.Cleanup(outFile.Name())

	cmd := echoCommand("foo")
	pipe, err := cmd.StdoutTeePipe(outFile)
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	err = cmd.Wait()
	require.NoError(t, err)

	outFromPipe, err := io.ReadAll(pipe)
	require.NoError(t, err)
	require.Equal(t, "foo", strings.TrimSpace(string(outFromPipe)))

	err = pipe.Close()
	require.NoError(t, err)

	err = outFile.Close()
	require.NoError(t, err)

	outFromFile, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Equal(t, "foo", strings.TrimSpace(string(outFromFile)))
}

func TestCmd_StdoutTeePipe_ReadSyncWithRun(t *testing.T) {
	// Pipe stdout to a file
	outFile, err := os.CreateTemp("", "outFile")
	require.NoError(t, err)
	defer fileutil.Cleanup(outFile.Name())

	cmd := echoCommand("foo")
	pipe, err := cmd.StdoutTeePipe(outFile)
	require.NoError(t, err)

	err = cmd.Run()
	require.NoError(t, err)

	outFromPipe, err := io.ReadAll(pipe)
	require.NoError(t, err)
	require.Equal(t, "foo", strings.TrimSpace(string(outFromPipe)))

	err = pipe.Close()
	require.NoError(t, err)

	err = outFile.Close()
	require.NoError(t, err)

	outFromFile, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Equal(t, "foo", strings.TrimSpace(string(outFromFile)))
}

func TestCmd_StdoutTeePipe_NoRead(t *testing.T) {
	// Pipe stdout to a file
	outFile, err := os.CreateTemp("", "outFile")
	require.NoError(t, err)
	defer fileutil.Cleanup(outFile.Name())

	cmd := echoCommand("foo")
	pipe, err := cmd.StdoutTeePipe(outFile)
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	err = cmd.Wait()
	require.NoError(t, err)

	err = outFile.Close()
	require.NoError(t, err)

	outFromFile, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Equal(t, "foo", strings.TrimSpace(string(outFromFile)))

	err = pipe.Close()
	require.NoError(t, err)
}

func TestCmd_StdoutTeePipe_NoReadWithRun(t *testing.T) {
	// Pipe stdout to a file
	outFile, err := os.CreateTemp("", "outFile")
	require.NoError(t, err)
	defer fileutil.Cleanup(outFile.Name())

	cmd := echoCommand("foo")
	pipe, err := cmd.StdoutTeePipe(outFile)
	require.NoError(t, err)

	err = cmd.Run()
	require.NoError(t, err)

	err = outFile.Close()
	require.NoError(t, err)

	outFromFile, err := os.ReadFile(outFile.Name())
	require.NoError(t, err)
	require.Equal(t, "foo", strings.TrimSpace(string(outFromFile)))

	err = pipe.Close()
	require.NoError(t, err)
}

func echoCommand(args ...string) *Cmd {
	if runtime.GOOS == "windows" {
		args = append([]string{"/d", "/c", "echo"}, args...)
		return Command("cmd.exe", args...)
	} else {
		return Command("echo", args...)
	}
}

func buildYes(t *testing.T) string {
	tmpDir := testutil.MkdirTemp(t, "", "cifuzz-yes-")
	path := filepath.Join(tmpDir, "yes")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	err := exec.Command("go", "build", "-o", path, "./testdata").Run()
	require.NoError(t, err)
	return path
}
