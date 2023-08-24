package e2e

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/pkg/cicheck"
)

const (
	ciServerToUseForE2ETests = "https://app.staging.code-intelligence.com"
	envvarWithE2EUserToken   = "E2E_TEST_CIFUZZ_API_TOKEN"
)

type Assertion func(*testing.T, CommandOutput)

type CommandOutput struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Stdall   string // Combined stdout and stderr output for simpler assertions
	Workdir  fs.FS  // Expose files from the test folder
	t        *testing.T
}

type CIUser string

const (
	AnonymousCIUser    CIUser = "anonymous"
	LoggedInCIUser     CIUser = "loggedIn"
	InvalidTokenCIUser CIUser = "invalidToken"
)

type TestCase struct {
	Description   string
	Command       string
	Environment   []string
	Args          []string
	SampleFolder  []string
	CIUser        CIUser
	ToolsRequired []string
	Assert        Assertion
	SkipOnOS      string
}

type testCaseRunOptions struct {
	command      string
	args         string
	sampleFolder string
}

// RunTests Runs all test cases generated from the input combinations
func RunTests(t *testing.T, testCases []TestCase) {
	TestUseLocalAPIToken(t)
	for _, testCase := range testCases { //nolint:gocritic
		runTest(t, &testCase)
	}
}

// runTest Generates 1...n tests from possible combinations in a TestCase.
func runTest(t *testing.T, testCase *TestCase) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	if cicheck.IsCIEnvironment() && os.Getenv("E2E_TESTS_MATRIX") == "" {
		t.Skip("Skipping e2e tests. You need to set E2E_TESTS_MATRIX envvar to run this test.")
	}

	if testCase.SkipOnOS == runtime.GOOS {
		t.Skip("Skipping e2e test. It is not supported on this OS.")
	}

	ctx := context.Background()
	dockerClient, err := getDockerClient()
	require.NoError(t, err)

	buildImageFromDockerFile(t, ctx, dockerClient, testCase)

	// Set defaults
	if len(testCase.Args) == 0 {
		testCase.Args = []string{""}
	}

	if len(testCase.SampleFolder) == 0 {
		testCase.SampleFolder = []string{"empty"}
	}

	if testCase.CIUser == "" {
		testCase.CIUser = AnonymousCIUser
	}

	if testCase.CIUser == LoggedInCIUser {
		if os.Getenv(envvarWithE2EUserToken) == "" {
			require.FailNow(t, "You are trying to test LoggedIn behavior, you need to set "+envvarWithE2EUserToken+" envvar.")
		}
		testCase.Environment = append(testCase.Environment, "CIFUZZ_API_TOKEN="+os.Getenv(envvarWithE2EUserToken))
	}

	if testCase.CIUser == InvalidTokenCIUser {
		testCase.Environment = append(testCase.Environment, "CIFUZZ_API_TOKEN=invalid")
	}

	// Generate all the combinations we want to test
	testCaseRuns := []testCaseRunOptions{}
	for _, args := range testCase.Args {
		for _, contextFolder := range testCase.SampleFolder {
			testCaseRuns = append(testCaseRuns, testCaseRunOptions{
				command:      testCase.Command,
				args:         args,
				sampleFolder: contextFolder,
			})
		}
	}

	for index, testCaseRun := range testCaseRuns {
		t.Run(fmt.Sprintf("[%d of %d] cifuzz %s %s", index+1, len(testCaseRuns), testCaseRun.command, testCaseRun.args), func(t *testing.T) {
			commandOutput := runTestCaseInContainer(t, ctx, dockerClient, testCase, testCaseRun)
			fmt.Println(commandOutput.Stdout)

			fmt.Println("")
			fmt.Println("---")
			fmt.Println("Exit code:", commandOutput.ExitCode)
			fmt.Println("---")

			fmt.Println("Stdout:")
			fmt.Println(commandOutput.Stdout)
			fmt.Println("---")

			fmt.Println("Stderr:")
			fmt.Println(commandOutput.Stderr)
			fmt.Println("---")
			testCase.Assert(t, commandOutput)
		})
	}
}
