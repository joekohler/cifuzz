package java

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/coverage"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/options"
	parser "code-intelligence.com/cifuzz/pkg/parser/coverage"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/envutil"
	"code-intelligence.com/cifuzz/util/executil"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type CoverageGenerator struct {
	BuildSystem  string
	OutputFormat string
	OutputPath   string
	FuzzTest     string
	TargetMethod string
	ProjectDir   string

	Deps       []string
	CorpusDirs []string
	EngineArgs []string

	BuildStdout io.Writer
	BuildStderr io.Writer
	Stderr      io.Writer
}

// BuildFuzzTestForCoverage builds the jacoco.exec file for
// the fuzz test which is used to generate the coverage report.
func (cov *CoverageGenerator) BuildFuzzTestForCoverage() error {
	if cov.OutputPath == "" {
		cov.OutputPath = filepath.Join(cov.ProjectDir, ".cifuzz-build", "report")
	}
	// Make sure that the directories actually exist otherwise
	// the java command later on will fail
	err := os.MkdirAll(cov.OutputPath, 0755)
	if err != nil {
		return errors.WithStack(err)
	}

	// Set the Java agent
	agentJar, err := runfiles.Finder.JacocoAgentJarPath()
	if err != nil {
		return err
	}

	return cov.produceJacocoExec(agentJar, cov.jacocoExecFilePath())
}

// GenerateCoverageReport creates a jacoco.xml report with the
// jacoco CLI and depending on the output format, also converts
// it to a html or lcov report.
func (cov *CoverageGenerator) GenerateCoverageReport() (string, error) {
	cliJar, err := runfiles.Finder.JacocoCLIJarPath()
	if err != nil {
		return "", err
	}

	// Class files are stored differently dependent on build system
	classFilesDir := filepath.Join(cov.ProjectDir, "target", "classes")
	if cov.BuildSystem == config.BuildSystemGradle {
		classFilesDir = filepath.Join(cov.ProjectDir, "build", "classes")
	}

	htmlPath := filepath.Join(cov.OutputPath, "html")
	jacocoXMLPath, err := cov.runJacocoCommand(cliJar, cov.jacocoExecFilePath(), htmlPath, classFilesDir)
	if err != nil {
		return "", err
	}

	jacocoReport, err := os.Open(jacocoXMLPath)
	if err != nil {
		return "", errors.WithStack(err)
	}

	parser.ParseJacocoXMLIntoSummary(jacocoReport).PrintTable(cov.Stderr)
	// Close the report here directly, so it can be used
	// for lcov parsing if needed
	jacocoReport.Close()

	switch cov.OutputFormat {
	case coverage.FormatJacocoXML:
		return jacocoXMLPath, nil
	case coverage.FormatHTML:
		return htmlPath, nil
	case coverage.FormatLCOV:
		// Open report here again otherwise it will be seen as empty
		// after parsing it into the summary
		reportFile, err := os.Open(jacocoXMLPath)
		if err != nil {
			return "", errors.WithStack(err)
		}

		lcovReport, err := parser.ParseJacocoXMLIntoLCOVReport(reportFile)
		if err != nil {
			return "", err
		}

		lcovFilePath := filepath.Join(cov.OutputPath, "report.lcov")
		err = lcovReport.WriteLCOVReportToFile(lcovFilePath)
		if err != nil {
			return "", err
		}

		return lcovFilePath, err
	}

	return "", fmt.Errorf("undefined output format: %s", cov.OutputFormat)
}

func (cov *CoverageGenerator) BuildFuzzTestForContainerCoverage(jacocoExecFilePath string) error {
	log.Info("Creating coverage report")

	err := os.MkdirAll(cov.OutputPath, 0755)
	if err != nil {
		return errors.WithStack(err)
	}

	// Find the JaCoCo Java agent JAR in the class paths
	jacocoAgentRuntimePattern := regexp.MustCompile(`^org\.jacoco\.agent-.*-runtime\.jar$`)
	var jacocoAgentJar string
	for _, classPath := range cov.Deps {
		if jacocoAgentRuntimePattern.MatchString(filepath.Base(classPath)) {
			jacocoAgentJar = classPath
			break
		}
	}
	if jacocoAgentJar == "" {
		return errors.New("JaCoCo agent JAR not found in class paths")
	}

	return cov.produceJacocoExec(jacocoAgentJar, jacocoExecFilePath)
}

func (cov *CoverageGenerator) GenerateCoverageReportInFuzzContainer(jacocoExecFilePath string) (string, error) {
	// Find the jacoco cli JAR in the class paths
	jacocoCLIPattern := regexp.MustCompile(`^org\.jacoco\.cli-.*\.jar$`)
	var cliJar string
	for _, dep := range cov.Deps {
		if jacocoCLIPattern.MatchString(filepath.Base(dep)) {
			cliJar = dep
			break
		}
	}
	if cliJar == "" {
		return "", errors.New("jacococli JAR not found in class paths")
	}

	classFilesDir := "/cifuzz/runtime_deps/target/classes"
	jacocoXMLFile, err := cov.runJacocoCommand(cliJar, jacocoExecFilePath, "", classFilesDir)
	if err != nil {
		return "", err
	}

	// Convert jacoco.xml report to LCOV report
	fileReader, err := os.Open(jacocoXMLFile)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer fileReader.Close()
	lcovReport, err := parser.ParseJacocoXMLIntoLCOVReport(fileReader)
	if err != nil {
		return "", err
	}

	// Remove jacoco.xml file because we don't need it anymore
	defer func() {
		err = os.Remove(jacocoXMLFile)
		if err != nil {
			log.Debugf("Failed to remove intermediate jacoco.xml report: %v", err)
		}
	}()

	// Write the LCOV report to the specified path.
	lcovPath := filepath.Join(cov.OutputPath, "report.lcov")
	err = lcovReport.WriteLCOVReportToFile(lcovPath)
	if err != nil {
		return "", err
	}

	log.Infof("Successfully created coverage report: %s", lcovPath)

	return lcovPath, nil
}

func (cov *CoverageGenerator) environment() ([]string, error) {
	var env []string
	var err error

	// Try to find a reasonable JAVA_HOME if none is set.
	if _, set := envutil.LookupEnv(env, "JAVA_HOME"); !set {
		javaHome, err := runfiles.Finder.JavaHomePath()
		if err != nil {
			return nil, err
		}
		env, err = envutil.Setenv(env, "JAVA_HOME", javaHome)
		if err != nil {
			return nil, err
		}
	}

	// Enable more verbose logging for Jazzer's libjvm.so search process
	env, err = envutil.Setenv(env, "RULES_JNI_TRACE", "1")
	if err != nil {
		return nil, err
	}

	return env, nil
}

// jacocoExecFilePath returns the path where the jacoco.exec should
// be generated. Including the fuzz test in the file name ensures
// that no aggregated reports are created.
func (cov *CoverageGenerator) jacocoExecFilePath() string {
	return filepath.Join(cov.OutputPath, fmt.Sprintf("jacoco_%s_%s.exec", cov.FuzzTest, cov.TargetMethod))
}

func (cov *CoverageGenerator) runJacocoCommand(cliJar, jacocoExecPath, htmlPath, classFilesDir string) (string, error) {
	jacocoXMLPath := filepath.Join(cov.OutputPath, "jacoco.xml")

	args := []string{
		"-jar", cliJar,
		"report", jacocoExecPath,
		"--xml", jacocoXMLPath,
		"--classfiles", classFilesDir,
	}
	// Set html output path if needed
	if cov.OutputFormat == coverage.FormatHTML {
		args = append(args, "--html", htmlPath)
	}

	// Produce a JaCoCo XML report from the jacoco.exec file
	cmd := executil.CommandContext(context.Background(), "java", args...)
	cmd.Stderr = cov.BuildStderr
	cmd.Stdout = cov.BuildStdout
	log.Debugf("Command: %s", strings.Join(stringutil.QuotedStrings(cmd.Args), " "))
	err := cmd.Run()
	if err != nil {
		return "", errors.WithStack(err)
	}

	return jacocoXMLPath, nil
}

func (cov *CoverageGenerator) produceJacocoExec(agentJarPath, jacocoExecFilePath string) error {
	javaBin, err := runfiles.Finder.JavaPath()
	if err != nil {
		return err
	}
	args := []string{javaBin}

	if err != nil {
		return err
	}
	args = append(args, fmt.Sprintf("-javaagent:%s=destfile=%s", agentJarPath, jacocoExecFilePath))

	// Get class path from dependencies
	classPath := strings.Join(cov.Deps, string(os.PathListSeparator))
	args = append(args, "-cp", classPath)

	// JVM tuning arguments
	// See https://github.com/CodeIntelligenceTesting/jazzer/blob/main/docs/common.md#recommended-jvm-options
	args = append(args,
		// Preserve and emit stack trace information even on hot paths.
		// This may hurt performance, but also helps find flaky bugs.
		"-XX:-OmitStackTraceInFastThrow",
		// Optimize GC for high throughput rather than low latency.
		"-XX:+UseParallelGC",
		// CriticalJNINatives has been removed in JDK 18.
		"-XX:+IgnoreUnrecognizedVMOptions",
		// Improves the performance of Jazzer's tracing hooks.
		"-XX:+CriticalJNINatives",
		// Disable warnings caused by the use of Jazzer's Java agent on JDK 21+.
		"-XX:+EnableDynamicAgentLoading",
	)

	// Jazzer main class
	args = append(args, options.JazzerMainClass)

	// Jazzer fuzz test options
	args = append(args, options.JazzerTargetClassFlag(cov.FuzzTest))
	args = append(args, options.JazzerTargetMethodFlag(cov.TargetMethod))

	// Tell Jazzer to not apply fuzzing instrumentation, because we only
	// want to run the inputs from the corpus directories to produce
	// coverage data.
	args = append(args, options.JazzerHooksFlag(false))

	// Tell Jazzer to continue on findings, because we want to collect
	// coverage for all the inputs and not stop on findings.
	args = append(args, options.JazzerKeepGoingFlag(math.MaxInt32))
	// The --keep_going flag requires --dedup to be set as well
	args = append(args, options.JazzerDedupFlag(true))

	// Tell libFuzzer to never stop on timeout
	// (but only when all inputs were used).
	args = append(args, options.LibFuzzerMaxTotalTimeFlag("0"))

	// Only run the inputs from the corpus directories
	args = append(args, "-runs=0")

	// Add any additional corpus directories
	// Jazzer uses the generated corpus automatically
	args = append(args, cov.CorpusDirs...)

	// Add engine args for jazzer
	args = append(args, cov.EngineArgs...)

	// Set the directory in which fuzzing artifacts (e.g. crashes) are
	// stored. This must be an absolute path, because else crash files
	// are created in the current working directory, which the fuzz test
	// could change.
	outputDir, err := os.MkdirTemp("", "jazzer-out-")
	if err != nil {
		return errors.WithStack(err)
	}
	defer fileutil.Cleanup(outputDir)
	args = append(args, options.LibFuzzerArtifactPrefixFlag(outputDir+"/"))

	env, err := cov.environment()
	if err != nil {
		return err
	}

	// Run Jazzer with the JaCoCo agent to produce a jacoco.exec file
	cmd := executil.CommandContext(context.Background(), args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdout = cov.BuildStdout
	cmd.Stderr = cov.BuildStderr
	log.Debugf("Command: %s", envutil.QuotedCommandWithEnv(cmd.Args, env))
	err = cmd.Run()
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}
