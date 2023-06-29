package maven

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/cmd/coverage/summary"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/executil"
)

func TestIntegration_Maven(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))

	// Copy testdata
	projectDir := shared.CopyTestdataDir(t, "maven")

	cifuzzRunner := shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  projectDir,
		DefaultFuzzTest: "com.example.FuzzTestCase",
	}

	// Execute the init command
	// The instructions file for maven includes dependencies, plugin and a profile section for jacoco that
	// need to be included at different locations in the pom.xml, so we split the instructions file up into
	// <dependency> and <profile> and ignore the code adding the jacoco plugin because the user should already
	// have it, the instruction is just a nudge so it doesn't get overlooked and doesn't need to be tested.
	linesToAdd := cifuzzRunner.CommandWithFilterForInstructions(t, "init", nil)
	assert.FileExists(t, filepath.Join(projectDir, "cifuzz.yaml"))
	shared.AddLinesToFileAtBreakPoint(t,
		filepath.Join(projectDir, "pom.xml"),
		strings.Split(strings.Split(strings.Join(linesToAdd, "\n"), "<plugin>")[0], "\n"),
		"    </dependencies>",
		false,
	)
	shared.AddLinesToFileAtBreakPoint(t,
		filepath.Join(projectDir, "pom.xml"),
		strings.Split("<profile>"+strings.Split(strings.Join(linesToAdd, "\n"), "<profile>")[1], "\n"),
		"    </profiles>",
		false,
	)

	// Execute the create command
	testDir := filepath.Join(
		"src",
		"test",
		"java",
		"com",
		"example",
	)
	err := os.MkdirAll(filepath.Join(projectDir, testDir), 0755)
	require.NoError(t, err)
	outputPath := filepath.Join(testDir, "FuzzTestCase.java")
	cifuzzRunner.CommandWithFilterForInstructions(t, "create", &shared.CommandOptions{
		Args: []string{"java", "--output", outputPath}},
	)

	// Check that the fuzz test was created in the correct directory
	fuzzTestPath := filepath.Join(projectDir, outputPath)
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
	expectedOutputExp := regexp.MustCompile(`High: Remote Code Execution`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})

	// Check that the findings command lists the finding
	findings = shared.GetFindings(t, cifuzz, projectDir)
	require.Len(t, findings, 1)
	require.Contains(t, findings[0].Details, "Remote Code Execution")

	expectedStackTrace := []*stacktrace.StackFrame{
		{
			SourceFile:  "com.example.ExploreMe",
			Line:        19,
			Column:      0,
			FrameNumber: 0,
			Function:    "exploreMe",
		},
	}

	require.Equal(t, expectedStackTrace, findings[0].StackTrace)

	// Check that options set via the config file are respected
	configFileContent := "print-json: true"
	err = os.WriteFile(filepath.Join(projectDir, "cifuzz.yaml"), []byte(configFileContent), 0644)
	require.NoError(t, err)
	expectedOutputExp = regexp.MustCompile(`"finding": {`)
	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: []*regexp.Regexp{expectedOutputExp},
	})

	// Check that command-line flags take precedence over config file
	// settings (only on Linux because we only support Minijail on
	// Linux).
	cifuzzRunner.Run(t, &shared.RunOptions{
		Args:             []string{"--json=false"},
		UnexpectedOutput: expectedOutputExp,
	})

	// Clear cifuzz.yml so that subsequent tests run with defaults (e.g. sandboxing).
	err = os.WriteFile(filepath.Join(projectDir, "cifuzz.yaml"), nil, 0644)
	require.NoError(t, err)

	// Produce a jacoco xml coverage report
	createJacocoXMLCoverageReport(t, cifuzz, projectDir)

	testCoverageVSCodePreset(t, cifuzz, projectDir)

	// Run cifuzz bundle and verify the contents of the archive.
	shared.TestBundleMaven(t, projectDir, cifuzz, "com.example.FuzzTestCase")

	// Check if adding additional jazzer parameters via flags is respected
	shared.TestAdditionalJazzerParameters(t, cifuzz, projectDir)
}

func createJacocoXMLCoverageReport(t *testing.T, cifuzz, dir string) {
	t.Helper()

	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "report", "com.example.FuzzTestCase")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "report", "jacoco.xml")
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for
	// ExploreMe.java source file, but not for App.java.
	reportFile, err := os.Open(reportPath)
	require.NoError(t, err)
	defer reportFile.Close()
	summary := summary.ParseJacocoXML(reportFile)
	for _, file := range summary.Files {
		if file.Filename == "com/example/ExploreMe.java" {
			assert.Equal(t, 2, file.Coverage.FunctionsHit)
			assert.Equal(t, 10, file.Coverage.LinesHit)
			assert.Equal(t, 8, file.Coverage.BranchesHit)

		} else if file.Filename == "com/example/App.java" {
			assert.Equal(t, 0, file.Coverage.FunctionsHit)
			assert.Equal(t, 0, file.Coverage.LinesHit)
			assert.Equal(t, 0, file.Coverage.BranchesHit)
		}
	}
}

func testCoverageVSCodePreset(t *testing.T, cifuzz, dir string) {
	t.Helper()

	cmd := executil.Command(cifuzz, "coverage",
		"-v",
		"--preset=vscode",
		"com.example.FuzzTestCase")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	reportPath := filepath.Join(dir, "report", "jacoco.xml")
	require.FileExists(t, reportPath)
}

func modifyFuzzTestToCallFunction(t *testing.T, fuzzTestPath string) {
	// Modify the fuzz test stub created by `cifuzz create` to actually
	// call a function.

	f, err := os.OpenFile(fuzzTestPath, os.O_RDWR, 0700)
	require.NoError(t, err)
	defer f.Close()
	scanner := bufio.NewScanner(f)

	var lines []string
	var seenBeginningOfFuzzTestFunc bool
	var addedFunctionCall bool
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "    @FuzzTest") {
			seenBeginningOfFuzzTestFunc = true
		}
		// Insert the function call at the end of the myFuzzTest
		// function, right above the "}".
		if seenBeginningOfFuzzTestFunc && strings.HasPrefix(scanner.Text(), "    }") {
			lines = append(lines, []string{
				"        int a = data.consumeInt();",
				"        int b = data.consumeInt();",
				"        String c = data.consumeRemainingAsString();",
				"		 ExploreMe ex = new ExploreMe(a);",
				"        ex.exploreMe(b, c);",
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
