package jazzer

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/options"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	fuzzer_runner "code-intelligence.com/cifuzz/pkg/runner"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/envutil"
)

type RunnerOptions struct {
	LibfuzzerOptions              *libfuzzer.RunnerOptions
	AutofuzzTarget                string
	TargetClass                   string
	TargetMethod                  string
	ClassPaths                    []string
	InstrumentationPackageFilters []string
}

func (options *RunnerOptions) ValidateOptions() error {
	err := options.LibfuzzerOptions.ValidateOptions()
	if err != nil {
		return err
	}

	if options.AutofuzzTarget == "" && options.TargetClass == "" {
		return errors.New("Either a autofuzz target or a target class must be specified")
	}
	if options.AutofuzzTarget != "" && options.TargetClass != "" {
		return errors.New("Only specify either an autofuzz target or a target class")
	}

	return nil
}

type Runner struct {
	*RunnerOptions
	*libfuzzer.Runner
}

func NewRunner(options *RunnerOptions) *Runner {
	libfuzzerRunner := libfuzzer.NewRunner(options.LibfuzzerOptions)
	libfuzzerRunner.SupportJazzer = true
	libfuzzerRunner.SupportJazzerJS = false
	return &Runner{options, libfuzzerRunner}
}

func (r *Runner) Run(ctx context.Context) error {
	err := r.ValidateOptions()
	if err != nil {
		return err
	}

	javaHome, err := runfiles.Finder.JavaHomePath()
	if err != nil {
		return err
	}

	javaBin := filepath.Join(javaHome, "bin", "java")
	if runtime.GOOS == "windows" {
		javaBin = filepath.Join(javaHome, "bin", "java.exe")
	}
	args := []string{javaBin}

	// class paths
	args = append(args, "-cp", strings.Join(r.ClassPaths, string(os.PathListSeparator)))

	// JVM tuning args
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

	// ----------------------
	// --- Jazzer options ---
	// ----------------------
	if r.AutofuzzTarget != "" {
		args = append(args, options.JazzerAutoFuzzFlag(r.AutofuzzTarget))
	} else {
		args = append(args, options.JazzerTargetClassFlag(r.TargetClass))
		args = append(args, options.JazzerTargetMethodFlag(r.TargetMethod))
	}
	// -------------------------
	// --- libfuzzer options ---
	// -------------------------
	// Tell libfuzzer to exit after the timeout but only add the argument if the timeout is not 0 otherwise it will
	// override jazzer's default timeout and never stop
	timeoutSeconds := int64(r.Timeout.Seconds())
	if timeoutSeconds > 0 {
		timeoutStr := strconv.FormatInt(timeoutSeconds, 10)
		args = append(args, options.LibFuzzerMaxTotalTimeFlag(timeoutStr))
	}

	// Tell libfuzzer which dictionary it should use
	if r.Dictionary != "" {
		args = append(args, options.LibFuzzerDictionaryFlag(r.Dictionary))
	}

	// Add user-specified Jazzer/libfuzzer options
	args = append(args, r.EngineArgs...)

	// Tell Jazzer which corpus directory it should use, if specified.
	// By default, Jazzer stores the generated corpus in
	// .cifuzz-corpus/<test class name>/<test method name>.
	if r.GeneratedCorpusDir != "" {
		args = append(args, r.GeneratedCorpusDir)
	}

	// Add any additional corpus directories as further positional arguments
	args = append(args, r.SeedCorpusDirs...)

	// The environment we run the fuzzer in
	env, err := r.FuzzerEnvironment()
	if err != nil {
		return err
	}

	return r.RunLibfuzzerAndReport(ctx, args, env)
}

func (r *Runner) FuzzerEnvironment() ([]string, error) {
	var env []string
	var err error

	env, err = fuzzer_runner.AddEnvFlags(env, r.EnvVars)
	if err != nil {
		return nil, err
	}

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

	// Enable more verbose logging for Jazzer's libjvm.so search process.
	env, err = envutil.Setenv(env, "RULES_JNI_TRACE", "1")
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (r *Runner) Cleanup(ctx context.Context) {
	r.Runner.Cleanup(ctx)
}
