package cmake

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/integration-tests/shared"
	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/parser/libfuzzer/stacktrace"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestIntegration_CMake(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// Install cifuzz
	testutil.RegisterTestDepOnCIFuzz()
	installDir := shared.InstallCIFuzzInTemp(t)
	t.Cleanup(func() { fileutil.Cleanup(installDir) })
	cifuzz := builderPkg.CIFuzzExecutablePath(filepath.Join(installDir, "bin"))
	// Include the CMake package by setting the CMAKE_PREFIX_PATH.
	err := os.Setenv("CMAKE_PREFIX_PATH", filepath.Join(installDir, "share", "cmake"))
	require.NoError(t, err)

	// Copy testdata
	dir := shared.CopyTestdataDir(t, "cmake")
	t.Cleanup(func() { fileutil.Cleanup(dir) })
	t.Logf("executing cmake integration test in %s", dir)

	cifuzzRunner := &shared.CIFuzzRunner{
		CIFuzzPath:      cifuzz,
		DefaultWorkDir:  dir,
		DefaultFuzzTest: "parser_fuzz_test",
	}

	// Execute the root command
	cifuzzRunner.CommandWithFilterForInstructions(t, "", nil)

	// Execute the init command
	linesToAdd := cifuzzRunner.CommandWithFilterForInstructions(t, "init", nil)
	shared.AddLinesToFileAtBreakPoint(t, filepath.Join(dir, "CMakeLists.txt"), linesToAdd, "add_subdirectory", false)

	// Execute the create command
	outputPath := filepath.Join("src", "parser", "parser_fuzz_test.cpp")
	linesToAdd = cifuzzRunner.CommandWithFilterForInstructions(t, "create", &shared.CommandOptions{
		Args: []string{"cpp", "--output", outputPath},
	})

	// Check that the fuzz test was created in the correct directory
	fuzzTestPath := filepath.Join(dir, outputPath)
	require.FileExists(t, fuzzTestPath)

	// Append the lines to CMakeLists.txt
	shared.AppendLines(t, filepath.Join(filepath.Dir(fuzzTestPath), "CMakeLists.txt"), linesToAdd)

	// Check that the findings command doesn't list any findings yet
	findings := shared.GetFindings(t, cifuzz, dir)
	require.Empty(t, findings)

	t.Run("runEmptyFuzzTest", func(t *testing.T) {
		// Run the (empty) fuzz test
		cifuzzRunner.Run(t, &shared.RunOptions{
			ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(`^paths: \d+`)},
			TerminateAfterExpectedOutput: true,
		})
	})

	// Make the fuzz test call a function. Before we do that, we sleep
	// for one second, to avoid make implementations which only look at
	// the full seconds of the timestamp to not rebuild the target, see
	// https://www.gnu.org/software/autoconf/manual/autoconf-2.63/html_node/Timestamps-and-Make.html
	time.Sleep(time.Second)
	shared.ModifyFuzzTestToCallFunction(t, fuzzTestPath)

	// Add dependency on parser lib to CMakeLists.txt
	cmakeLists := filepath.Join(filepath.Dir(fuzzTestPath), "CMakeLists.txt")
	shared.AppendLines(t, cmakeLists, []string{"target_link_libraries(parser_fuzz_test PRIVATE parser)"})

	t.Run("runBuildOnly", func(t *testing.T) {
		cifuzzRunner.Run(t, &shared.RunOptions{Args: []string{"--build-only"}})
	})

	t.Run("runWithAdditionalArgs", func(t *testing.T) {
		testRunWithAdditionalArgs(t, cifuzzRunner)
	})

	t.Run("run", func(t *testing.T) {
		testRun(t, cifuzzRunner)

		t.Run("htmlReport", func(t *testing.T) {
			// Produce a coverage report for parser_fuzz_test
			testHTMLCoverageReport(t, cifuzz, dir)
		})
		t.Run("lcovReport", func(t *testing.T) {
			// Produces a coverage report for crashing_fuzz_test
			testLcovCoverageReport(t, cifuzz, dir)
		})
		t.Run("coverageWithAdditionalArgs", func(t *testing.T) {
			// Run cifuzz coverage with additional args
			testCoverageWithAdditionalArgs(t, cifuzz, dir)
		})
		t.Run("coverageVSCodePreset", func(t *testing.T) {
			testCoverageVSCodePreset(t, cifuzz, dir)
		})
	})

	t.Run("runWithConfigFile", func(t *testing.T) {
		// Check that options set via the config file are respected
		testRunWithConfigFile(t, cifuzzRunner)
	})

	t.Run("runWithAsanOptions", func(t *testing.T) {
		// Check that ASAN_OPTIONS can be set
		testRunWithAsanOptions(t, cifuzzRunner)
	})

	t.Run("runWithSecretEnvVar", func(t *testing.T) {
		testRunWithSecretEnvVar(t, cifuzzRunner)
	})

	t.Run("bundle", func(t *testing.T) {
		if runtime.GOOS != "linux" && !config.AllowUnsupportedPlatforms() {
			t.Skip("Creating a bundle for CMake is currently only supported on Linux")
		}
		// Run cifuzz bundle and verify the contents of the archive.
		shared.TestBundleLibFuzzer(t, dir, cifuzz, os.Environ(), "parser_fuzz_test")

		// Run cifuzz bundle with additional args
		testBundleWithAdditionalArgs(t, cifuzz, dir)

		testBundleWithAddArg(t, cifuzz, dir)
	})

	t.Run("remoteRun", func(t *testing.T) {
		if runtime.GOOS != "linux" && !config.AllowUnsupportedPlatforms() {
			t.Skip("The remote-run command is currently only supported on Linux")
		}
		testRemoteRun(t, cifuzzRunner)

		testRemoteRunWithAdditionalArgs(t, cifuzzRunner)
	})

	t.Run("runWithUpload", func(t *testing.T) {
		testRunWithUpload(t, cifuzzRunner)
	})

	t.Run("runNotAuthenticated", func(t *testing.T) {
		testRunNotAuthenticated(t, cifuzzRunner)
	})
}

func testCoverageWithAdditionalArgs(t *testing.T, cifuzz string, dir string) {
	// Skip this test if we're running on a PRERELEASE pipeline, because
	// we might append additional arguments to the command, which might
	// cause this test to fail.
	if os.Getenv("CIFUZZ_PRERELEASE") != "" {
		t.Skip("skipping testRunWithAdditionalArgs")
	}

	// Run cmake and expect it to fail because we passed it a non-existent flag
	cmd := executil.Command(cifuzz, "coverage", "parser_fuzz_test", "--", "--non-existent-flag")
	cmd.Dir = dir

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	output, err := cmd.CombinedOutput()
	regexp := regexp.MustCompile("Unknown argument --non-existent-flag")
	seenExpectedOutput := regexp.MatchString(string(output))
	require.Error(t, err)
	require.True(t, seenExpectedOutput)
}

func testBundleWithAddArg(t *testing.T, cifuzz string, dir string) {
	viper.Set("verbose", true)

	test := []struct {
		param  string
		expect string
	}{
		{param: "--add=add_me.txt", expect: filepath.Join("work_dir", "add_me.txt")},
		{param: "--add=add_me.txt;rename.txt", expect: "rename.txt"},
		{param: "--add=add_me.txt;my_dir/add_me.txt", expect: filepath.Join("my_dir", "add_me.txt")},
	}

	args := []string{"bundle", "parser_fuzz_test"}
	for _, tc := range test {
		args = append(args, tc.param)
	}
	cmd := executil.Command(cifuzz, args...)
	cmd.Dir = dir
	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	// execute command
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(output), "Successfully created bundle: parser_fuzz_test.tar.gz")

	// extract bundle archive and check for expected files
	bundlePath := filepath.Join(dir, "parser_fuzz_test.tar.gz")
	archiveDir, err := os.MkdirTemp("", "cmake-bundle-extracted-archive-*")
	require.NoError(t, err)
	defer fileutil.Cleanup(archiveDir)

	err = archive.ExtractArchiveForTestsOnly(bundlePath, archiveDir)
	require.NoError(t, err)

	for _, tc := range test {
		assert.FileExists(t, filepath.Join(archiveDir, tc.expect))
	}
}

func testBundleWithAdditionalArgs(t *testing.T, cifuzz string, dir string) {
	// Skip this test if we're running on a PRERELEASE pipeline, because
	// we might append additional arguments to the command, which might
	// cause this test to fail.
	if os.Getenv("CIFUZZ_PRERELEASE") != "" {
		t.Skip("skipping testRunWithAdditionalArgs")
	}

	// Run cmake and expect it to fail because we passed it a non-existent flag
	cmd := executil.Command(cifuzz, "bundle", "parser_fuzz_test", "--", "--non-existent-flag")
	cmd.Dir = dir

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	output, err := cmd.CombinedOutput()
	regexp := regexp.MustCompile("Unknown argument --non-existent-flag")
	seenExpectedOutput := regexp.MatchString(string(output))
	require.Error(t, err)
	require.True(t, seenExpectedOutput)
}

func testRunWithAdditionalArgs(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	// Skip this test if we're running on a PRERELEASE pipeline, because
	// we might append additional arguments to the command, which might
	// cause this test to fail.
	if os.Getenv("CIFUZZ_PRERELEASE") != "" {
		t.Skip("skipping testRunWithAdditionalArgs")
	}

	// Run cmake and expect it to fail because we passed it a non-existent flag
	cifuzzRunner.Run(t, &shared.RunOptions{
		Args: []string{"--", "--non-existent-flag"},
		ExpectedOutputs: []*regexp.Regexp{
			regexp.MustCompile(`Unknown argument --non-existent-flag`),
		},
		ExpectError: true,
	})
}

func testRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir

	// Run the fuzz test and check that it finds the undefined behavior
	// and the use-after-free.
	expectedOutputs := []*regexp.Regexp{
		regexp.MustCompile(`^==\d*==ERROR: AddressSanitizer: heap-use-after-free`),
	}
	expectedOutputs = append(expectedOutputs, regexp.MustCompile(`^SUMMARY: UndefinedBehaviorSanitizer: undefined-behavior`))

	// Check that Minijail is used (if running on Linux, because Minijail
	// is only supported on Linux)
	if runtime.GOOS == "linux" {
		expectedOutputs = append(expectedOutputs, regexp.MustCompile(`bin/minijail0`))
	}

	cifuzzRunner.Run(t, &shared.RunOptions{
		ExpectedOutputs: expectedOutputs,
	})

	// Check that the findings command lists the findings
	findings := shared.GetFindings(t, cifuzz, testdata)
	require.Len(t, findings, 2)
	var asanFinding *finding.Finding
	var ubsanFinding *finding.Finding
	for _, f := range findings {
		if strings.HasPrefix(f.Details, "heap-use-after-free") {
			asanFinding = f
		} else if strings.HasPrefix(f.Details, "undefined behavior") {
			ubsanFinding = f
		} else {
			t.Fatalf("unexpected finding: %q", f.Details)
		}
	}

	// Verify that there is an ASan finding and that it has the correct details.
	require.NotNil(t, asanFinding)
	// Verify that ASan findings come with inputs under the project directory.
	require.NotEmpty(t, asanFinding.InputFile)
	require.False(t, filepath.IsAbs(asanFinding.InputFile), "Should be relative: %s", asanFinding.InputFile)
	require.FileExists(t, filepath.Join(testdata, asanFinding.InputFile))
	// TODO: This check currently fails on macOS because there
	// llvm-symbolizer doesn't read debug info from object files.
	// See https://github.com/google/sanitizers/issues/207#issuecomment-136495556
	if runtime.GOOS != "darwin" {
		expectedStackTrace := []*stacktrace.StackFrame{
			{
				SourceFile:  "src/parser/parser.cpp",
				Line:        19,
				Column:      14,
				FrameNumber: 0,
				Function:    "parse",
			},
			{
				SourceFile:  "src/parser/parser_fuzz_test.cpp",
				Line:        29,
				Column:      3,
				FrameNumber: 1,
				Function:    "LLVMFuzzerTestOneInputNoReturn",
			},
		}
		if runtime.GOOS == "windows" {
			expectedStackTrace[1].Line = 11
			expectedStackTrace[1].Function = "LLVMFuzzerTestOneInput"
			// On Windows, the column is not printed
			for i := range expectedStackTrace {
				expectedStackTrace[i].Column = 0
			}
		}

		require.Equal(t, expectedStackTrace, asanFinding.StackTrace)
	}

	// Verify that there is a UBSan finding and that it has the correct details.
	require.NotNil(t, ubsanFinding)
	// Verify that UBSan findings come with inputs under the project directory.
	require.NotEmpty(t, ubsanFinding.InputFile)
	require.False(t, filepath.IsAbs(ubsanFinding.InputFile), "Should be relative: %s", ubsanFinding.InputFile)
	require.FileExists(t, filepath.Join(testdata, ubsanFinding.InputFile))
	if runtime.GOOS != "darwin" {
		expectedStackTrace := []*stacktrace.StackFrame{
			{
				SourceFile:  "src/parser/parser.cpp",
				Line:        23,
				Column:      9,
				FrameNumber: 0,
				Function:    "parse",
			},
			{
				SourceFile:  "src/parser/parser_fuzz_test.cpp",
				Line:        29,
				Column:      3,
				FrameNumber: 1,
				Function:    "LLVMFuzzerTestOneInputNoReturn",
			},
		}
		if runtime.GOOS == "windows" {
			expectedStackTrace[1].Line = 11
			expectedStackTrace[1].Function = "LLVMFuzzerTestOneInput"
			// On Windows, the column is not printed
			for i := range expectedStackTrace {
				expectedStackTrace[i].Column = 0
			}
		}

		require.Equal(t, expectedStackTrace, ubsanFinding.StackTrace)
	}
}

func testRunWithAsanOptions(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	env, err := envutil.Setenv(os.Environ(), "ASAN_OPTIONS", "print_stats=1:atexit=1")
	require.NoError(t, err)
	cifuzzRunner.Run(t, &shared.RunOptions{
		Env:                          env,
		ExpectedOutputs:              []*regexp.Regexp{regexp.MustCompile(`Stats:`)},
		TerminateAfterExpectedOutput: false,
	})
}

func testRunWithSecretEnvVar(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	env, err := envutil.Setenv(os.Environ(), "SECRET", "verysecret")
	require.NoError(t, err)
	cifuzzRunner.Run(t, &shared.RunOptions{
		Env:              env,
		UnexpectedOutput: regexp.MustCompile(`verysecret`),
	})
}

func testRunWithConfigFile(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	testdata := cifuzzRunner.DefaultWorkDir

	configFileContent := `use-sandbox: false`
	err := os.WriteFile(filepath.Join(testdata, "cifuzz.yaml"), []byte(configFileContent), 0o644)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Clear cifuzz.yml so that subsequent tests run with defaults (e.g. sandboxing).
		err = os.WriteFile(filepath.Join(testdata, "cifuzz.yaml"), nil, 0o644)
		assert.NoError(t, err)
	})

	// Check that Minijail is not used (i.e. the artifact prefix is
	// not set to the Minijail output path)
	expectedOutputs := []*regexp.Regexp{
		regexp.MustCompile(regexp.QuoteMeta(`artifact_prefix='` + filepath.Join(os.TempDir(), "libfuzzer-out"))),
	}
	cifuzzRunner.Run(t, &shared.RunOptions{ExpectedOutputs: expectedOutputs})

	t.Run("WithFlags", func(t *testing.T) {
		// Check that command-line flags take precedence over config file
		// settings (only on Linux because we only support Minijail on
		// Linux).
		// TODO: We should use a different flag to also be able to test
		// this on non-Linux platforms
		if runtime.GOOS != "linux" {
			t.Skip()
		}
		cifuzzRunner.Run(t, &shared.RunOptions{
			Args:            []string{"--use-sandbox=true"},
			ExpectedOutputs: []*regexp.Regexp{regexp.MustCompile(`minijail`)},
		})
	})
}

func testHTMLCoverageReport(t *testing.T, cifuzz string, dir string) {
	cmd := executil.Command(cifuzz, "coverage", "-v",
		"--output", "coverage-report",
		"parser_fuzz_test")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	err := cmd.Run()
	require.NoError(t, err)

	reportPath := filepath.Join(dir, "coverage-report", "src", "parser", "index.html")
	if runtime.GOOS == "windows" {
		// On Windows the prefix flag is not working, which results in a slightly different reportPath
		reportPath = filepath.Join(dir, "coverage-report", "parser", "index.html")
	}
	require.FileExists(t, reportPath)

	// Check that the coverage report contains coverage for the
	// parser.cpp source file, but not for our headers.
	reportBytes, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	report := string(reportBytes)
	require.Contains(t, report, "parser.cpp")
	require.NotContains(t, report, "include/cifuzz")
}

func testLcovCoverageReport(t *testing.T, cifuzz string, dir string) {
	testCases := map[string]struct {
		sourceFiles            []string
		expectedLinesUncovered []uint
	}{
		"parser_fuzz_test": {
			sourceFiles: []string{
				filepath.Join("src", "parser", "parser.cpp"),
				filepath.Join("src", "parser", "parser_fuzz_test.cpp"),
			},
			// All lines should be covered
			expectedLinesUncovered: []uint{},
		},
		"crashing_fuzz_test": {
			sourceFiles: []string{
				filepath.Join("coverage", "crashing_fuzz_test.cpp"),
			},
			// Lines after the three crashes. Whether these are covered depends on
			// implementation details of the coverage instrumentation, so we
			// conservatively assume they aren't covered.
			expectedLinesUncovered: []uint{21, 31, 41},
		},
	}

	for testCase, testData := range testCases {
		// LLVM's continuous mode is not supported on Windows
		if runtime.GOOS == "windows" && testCase == "crashing_fuzz_test" {
			continue
		}

		reportPath := filepath.Join(dir, testCase+".lcov")

		cmd := executil.Command(cifuzz, "coverage", "-v",
			"--format=lcov",
			"--output", reportPath,
			testCase)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Terminate the cifuzz process when we receive a termination signal
		// (else the test won't stop).
		shared.TerminateOnSignal(t, cmd)

		err := cmd.Run()
		require.NoError(t, err)

		// Check that the coverage report was created
		require.FileExists(t, reportPath)

		// Read the report and extract all uncovered lines in the fuzz test source file.
		reportBytes, err := os.ReadFile(reportPath)
		require.NoError(t, err)
		lcov := bufio.NewScanner(bytes.NewBuffer(reportBytes))
		isFuzzTestSource := false
		var uncoveredLines []uint
		for lcov.Scan() {
			line := lcov.Text()

			if strings.HasPrefix(line, "SF:") {
				isFuzzTestSource = false
				for _, sourceFile := range testData.sourceFiles {
					if strings.HasSuffix(line, sourceFile) {
						isFuzzTestSource = true
					}
				}
				if !isFuzzTestSource {
					assert.Fail(t, "Unexpected source file: "+line)
				}
			}

			if !isFuzzTestSource || !strings.HasPrefix(line, "DA:") {
				continue
			}
			split := strings.Split(strings.TrimPrefix(line, "DA:"), ",")
			require.Len(t, split, 2)
			if split[1] == "0" {
				lineNo, err := strconv.Atoi(split[0])
				require.NoError(t, err)
				uncoveredLines = append(uncoveredLines, uint(lineNo))
			}
		}

		assert.Subset(t, testData.expectedLinesUncovered, uncoveredLines)
	}
}

func testRemoteRun(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir
	shared.TestRemoteRun(t, testdata, cifuzz)
}

func testRemoteRunWithAdditionalArgs(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir
	regexp := regexp.MustCompile("Unknown argument --non-existent-flag")
	shared.TestRemoteRunWithAdditionalArgs(t, testdata, cifuzz, regexp)
}

func testRunWithUpload(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir
	shared.TestRunWithUpload(t, testdata, cifuzz)
}

func testRunNotAuthenticated(t *testing.T, cifuzzRunner *shared.CIFuzzRunner) {
	cifuzz := cifuzzRunner.CIFuzzPath
	testdata := cifuzzRunner.DefaultWorkDir
	shared.TestRunNotAuthenticated(t, testdata, cifuzz)
}

func testCoverageVSCodePreset(t *testing.T, cifuzz, dir string) {
	reportPath := filepath.Join(dir, "lcov.info")

	cmd := executil.Command(cifuzz, "coverage",
		"-v",
		"--preset=vscode",
		"crashing_fuzz_test")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Terminate the cifuzz process when we receive a termination signal
	// (else the test won't stop).
	shared.TerminateOnSignal(t, cmd)

	err := cmd.Run()
	require.NoError(t, err)

	// Check that the coverage report was created
	require.FileExists(t, reportPath)
}
