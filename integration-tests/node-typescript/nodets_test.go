package nodets

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_NodeJS_TypeScript_InitCreateRun(t *testing.T) {
	if testing.Short() || os.Getenv("CIFUZZ_PRERELEASE") == "" {
		t.Skip()
	}

	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	t.Cleanup(func() { fileutil.Cleanup(installDir) })
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Copy testdata
	projectDir := shared.CopyTestdataDir(t, "node-typescript")
	defer fileutil.Cleanup(projectDir)

	cifuzzRunner := shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  projectDir,
		DefaultFuzzTest: "FuzzTestCase",
	}

	// Execute the init command
	instructions := cifuzzRunner.CommandWithFilterForInstructions(t, "init", &shared.CommandOptions{
		Env:  append(os.Environ(), "CIFUZZ_PRERELEASE=1"),
		Args: []string{"ts"},
	})
	require.FileExists(t, filepath.Join(projectDir, "cifuzz.yaml"))

	// Execute npm install --save-dev @jazzer.js/jest-runner
	npmArgs := getNpmArgs(t, instructions)
	require.NotEmpty(t, npmArgs)

	cmd := exec.Command("npm", npmArgs...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	t.Logf("Command: %s", cmd.String())
	err := cmd.Run()
	require.NoError(t, err)

	// Create jest.config.ts file
	jestConfig := getJestConfig(t, instructions)
	jestConfigPath := filepath.Join(projectDir, "jest.config.ts")
	err = os.WriteFile(jestConfigPath, []byte(jestConfig), 0o644)
	require.NoError(t, err)

	// Execute the create command
	fuzzTestPath := filepath.Join(projectDir, "FuzzTestCase.fuzz.ts")
	cifuzzRunner.CommandWithFilterForInstructions(t, "create", &shared.CommandOptions{
		Env:  append(os.Environ(), "CIFUZZ_PRERELEASE=1"),
		Args: []string{"ts", "--output", fuzzTestPath},
	},
	)

	// Check that the fuzz test was created in the correct directory
	require.FileExists(t, fuzzTestPath)

	// Check that the findings command doesn't list any findings yet
	findings := shared.GetFindings(t, cifuzz, projectDir)
	require.Empty(t, findings)

	// Run the (empty) fuzz test
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(`^paths: \d+`)},
		TerminateAfterExpectedOutput: true,
	})

	// Make the fuzz test call a function
	modifyFuzzTestToCallFunction(t, fuzzTestPath)
	// Run the fuzz test
	expectedOutputExp := regexp.MustCompile("Crash!")
	// setting -max_len to 8192 in ".jazzerjsrc" file to test if the file is read and picked up
	unexpectedOutputExp := regexp.MustCompile("INFO: -max_len is not provided; libFuzzer will not generate inputs larger than 4096 bytes")
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs:  []*regexp.Regexp{expectedOutputExp},
		UnexpectedOutput: unexpectedOutputExp,
	})

	// Check that the findings command lists the finding
	findings = shared.GetFindings(t, cifuzz, projectDir)
	require.Len(t, findings, 1)
	require.Contains(t, findings[0].Details, "Crash!")

	expectedStackTrace := []*stacktrace.StackFrame{
		{
			SourceFile:  "ExploreMe.ts",
			Line:        6,
			Column:      12,
			FrameNumber: 0,
			Function:    "exploreMe",
		},
	}
	require.Equal(t, expectedStackTrace, findings[0].StackTrace)
}

func getNpmArgs(t *testing.T, instructions []string) []string {
	t.Helper()

	for _, instruction := range instructions {
		if strings.HasPrefix(instruction, "npm") {
			args := strings.TrimSpace(strings.TrimPrefix(instruction, "npm"))
			return strings.Split(args, " ")
		}
	}

	return nil
}

func getJestConfig(t *testing.T, instructions []string) string {
	t.Helper()

	for i, instruction := range instructions {
		if strings.HasPrefix(instruction, "import type") {
			return strings.Join(instructions[i:len(instructions)-1], "\n")
		}
	}

	return ""
}

// Modify the fuzz test stub created by `cifuzz create` to actually call a function.
func modifyFuzzTestToCallFunction(t *testing.T, fuzzTestPath string) {
	f, err := os.OpenFile(fuzzTestPath, os.O_RDWR, 0o700)
	require.NoError(t, err)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// At the top of the file we add the required headers
	lines := []string{
		`import { exploreMe } from "./ExploreMe";`,
	}
	var seenBeginningOfFuzzTestFunc bool
	var addedFunctionCall bool
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "test.fuzz(") {
			seenBeginningOfFuzzTestFunc = true
		}
		// Insert the function call at the end of the FUZZ_TEST
		// function, right above the "}".
		if seenBeginningOfFuzzTestFunc && strings.HasPrefix(scanner.Text(), "}") {
			lines = append(lines, []string{
				"  const provider: FuzzedDataProvider = new FuzzedDataProvider(data);",
				"  const a: number = provider.consumeNumber();",
				"  const b: number = provider.consumeNumber();",
				"  const c: string = provider.consumeString(8);",
				"  exploreMe(a, b, c);",
			}...)
			addedFunctionCall = true
		}
		lines = append(lines, scanner.Text())
	}
	require.NoError(t, scanner.Err())
	require.True(t, addedFunctionCall)

	// Write the new content of the fuzz test back to file
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	_, err = f.WriteString(strings.Join(lines, "\n"))
	require.NoError(t, err)
}
