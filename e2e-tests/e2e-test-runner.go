package e2e

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
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
	Description  string
	Command      string
	Environment  []string
	Args         []string
	SampleFolder []string
	CIUser       CIUser
	// Os        []OSRuntime // When we will have tests depending on the OS
	// ToolsRequired []string # TODO: depending on the tools we will want to test
	Assert Assertion
}

func (co *CommandOutput) Success() *CommandOutput {
	assert.EqualValues(co.t, 0, co.ExitCode)
	return co
}

func (co *CommandOutput) Failed() *CommandOutput {
	assert.NotEqualValues(co.t, 0, co.ExitCode)
	return co
}

func (co *CommandOutput) OutputContains(expected string) *CommandOutput {
	assert.Contains(co.t, co.Stdout, expected)
	return co
}

func (co *CommandOutput) OutputNotContains(expected string) *CommandOutput {
	assert.NotContains(co.t, co.Stdout, expected)
	return co
}

func (co *CommandOutput) ErrorContains(expected string) *CommandOutput {
	assert.Contains(co.t, co.Stderr, expected)
	return co
}

func (co *CommandOutput) NoOutput() *CommandOutput {
	assert.Empty(co.t, co.Stdout)
	return co
}

func (co *CommandOutput) NoError() *CommandOutput {
	assert.Empty(co.t, co.Stderr)
	return co
}

type testCaseRunOptions struct {
	command      string
	args         string
	sampleFolder string
}

// RunTests Runs all test cases generated from the input combinations
func RunTests(t *testing.T, testCases []TestCase) {
	// Convenience option for local testing. Grab local token, backup the existing one and restore it after the test.
	if !cicheck.IsCIEnvironment() && os.Getenv(envvarWithE2EUserToken) == "" {
		fmt.Println("E2E_TEST_CIFUZZ_API_TOKEN envvar is not set. Trying to use the default one, since this is not a CI/CD run.")
		token := auth.GetToken(ciServerToUseForE2ETests)
		if token != "" {
			fmt.Println("Found local token with login.GetToken, going to use it for the tests.")
			t.Setenv(envvarWithE2EUserToken, token)
			tokenFilePath := tokenstorage.GetTokenFilePath()
			if _, err := os.Stat(tokenFilePath); err == nil {
				fmt.Println("Backing up existing access token file")
				_ = os.Rename(tokenFilePath, tokenFilePath+".bak")
				defer func() {
					_ = os.Rename(tokenFilePath+".bak", tokenFilePath)
				}()
			}
		}
	}
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
			fmt.Println("You are trying to test LoggedIn behavior, you need to set " + envvarWithE2EUserToken + " envvar.")
			t.FailNow()
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
		t.Run(fmt.Sprintf("[%d/%d] cifuzz %s %s", index+1, len(testCaseRuns), testCaseRun.command, testCaseRun.args), func(t *testing.T) {
			if testCase.CIUser != AnonymousCIUser {
				testCaseRun.args = "--server=" + ciServerToUseForE2ETests + " " + testCaseRun.args
			}
			fmt.Println("Running test:", testCase.Description)
			fmt.Println("Command:", "cifuzz", testCaseRun.command, testCaseRun.args)
			fmt.Println(" ")

			contextFolder := shared.CopyTestdataDirForE2E(t, testCaseRun.sampleFolder)

			// exec.Cmd can't handle empty args
			var cmd *exec.Cmd
			if len(testCaseRun.args) > 0 {
				argsSplice := deleteEmpty(append([]string{testCaseRun.command}, strings.Split(testCaseRun.args, " ")...))
				cmd = exec.Command("cifuzz", argsSplice...)
			} else {
				cmd = exec.Command("cifuzz", testCaseRun.command)
			}

			// add env vars
			cmd.Env = append(cmd.Env, testCase.Environment...)

			cmd.Dir = contextFolder

			stdout := bytes.Buffer{}
			errout := bytes.Buffer{}
			cmd.Stdout = &stdout
			cmd.Stderr = &errout

			err := cmd.Run()
			if err != nil {
				log.Printf("Error running command: %v", err)
			}

			fmt.Println("")
			fmt.Println("---")
			fmt.Println("Exit code:", cmd.ProcessState.ExitCode())
			fmt.Println("---")

			fmt.Println("Stdout:")
			fmt.Println(stdout.String())
			fmt.Println("---")

			fmt.Println("Stderr:")
			fmt.Println(errout.String())
			fmt.Println("---")
			testCase.Assert(t, CommandOutput{
				ExitCode: cmd.ProcessState.ExitCode(),
				Stdout:   stdout.String(),
				Stderr:   errout.String(),
				Stdall:   stdout.String() + errout.String(),
				Workdir:  os.DirFS(contextFolder),
				t:        t,
			})
		})
	}
}

func deleteEmpty(s []string) []string {
	var r []string
	for _, str := range s {
		if str != "" {
			r = append(r, str)
		}
	}
	return r
}
