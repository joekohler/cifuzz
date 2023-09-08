package runner

import (
	"context"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/java"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/logging"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/ldd"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runner/jazzer"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/fileutil"
)

type Runner interface {
	Run(*RunOptions, *reporthandler.ReportHandler) error
	CheckDependencies(string) error
}

type FuzzerRunner interface {
	Run(context.Context) error
	Cleanup(context.Context)
}

func prepareCorpusDir(opts *RunOptions, buildResult *build.BuildResult, reportHandler *reporthandler.ReportHandler) error {
	switch opts.BuildSystem {
	case config.BuildSystemCMake, config.BuildSystemBazel, config.BuildSystemOther:
		// The generated corpus dir has to be created before starting the fuzzing run.
		err := os.MkdirAll(buildResult.GeneratedCorpus, 0o755)
		if err != nil {
			return errors.WithStack(err)
		}
		log.Infof("Storing generated corpus in %s", fileutil.PrettifyPath(buildResult.GeneratedCorpus))

		// Ensure that symlinks are resolved to be able to add minijail
		// bindings for the corpus dirs.
		exists, err := fileutil.Exists(buildResult.SeedCorpus)
		if err != nil {
			return err
		}
		if exists {
			buildResult.SeedCorpus, err = filepath.EvalSymlinks(buildResult.SeedCorpus)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		buildResult.GeneratedCorpus, err = filepath.EvalSymlinks(buildResult.GeneratedCorpus)
		if err != nil {
			return errors.WithStack(err)
		}

		for i, dir := range opts.SeedCorpusDirs {
			opts.SeedCorpusDirs[i], err = filepath.EvalSymlinks(dir)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	case config.BuildSystemMaven, config.BuildSystemGradle:
		// The seed corpus dir has to be created before starting the fuzzing run.
		// Otherwise jazzer will store the findings in the project dir.
		// It is not necessary to create the corpus dir. Jazzer will do that for us.
		err := os.MkdirAll(cmdutils.JazzerSeedCorpus(opts.FuzzTest, opts.ProjectDir), 0o755)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	reportHandler.ManagedSeedCorpusDir = buildResult.SeedCorpus
	reportHandler.GeneratedCorpusDir = buildResult.GeneratedCorpus

	return nil
}

func ExecuteRunner(runner FuzzerRunner) error {
	// Handle cleanup (terminating the fuzzer process) when receiving
	// termination signals
	signalHandlerCtx, cancelSignalHandler := context.WithCancel(context.Background())
	routines, routinesCtx := errgroup.WithContext(signalHandlerCtx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	var signalErr error
	routines.Go(func() error {
		select {
		case <-routinesCtx.Done():
			return nil
		case s := <-sigs:
			log.Warnf("Received %s", s.String())
			signalErr = cmdutils.NewSignalError(s.(syscall.Signal))
			runner.Cleanup(routinesCtx)
			return signalErr
		}
	})

	// Run the fuzzer
	routines.Go(func() error {
		defer cancelSignalHandler()
		return runner.Run(routinesCtx)
	})

	err := routines.Wait()
	// We use a separate variable to pass signal errors, because when
	// a signal was received, the first goroutine terminates the second
	// one, resulting in a race of which returns an error first. In that
	// case, we always want to print the signal error, not the
	// "Unexpected exit code" error from the runner.
	if signalErr != nil {
		return signalErr
	}

	var execErr *cmdutils.ExecError
	if errors.As(err, &execErr) {
		// If the error is expected because libFuzzer might fail due to user
		// configuration, we return the execErr directly
		return execErr
	}

	// Routines.Wait() returns an error created by us so it already has a
	// stack trace and we don't want to add another one here
	// nolint: wrapcheck
	return err
}

func runLibfuzzer(opts *RunOptions, buildResult *build.BuildResult, reportHandler *reporthandler.ReportHandler) error {
	var err error

	style := pterm.Style{pterm.Reset, pterm.FgLightBlue}
	log.Infof("Running %s", style.Sprintf(opts.FuzzTest))
	log.Debugf("Executable: %s", buildResult.Executable)

	var libraryPaths []string
	if runtime.GOOS != "windows" {
		var err error
		libraryPaths, err = ldd.LibraryPaths(buildResult.Executable)
		if err != nil {
			return err
		}
	}

	// Use user-specified seed corpus dirs (if any) and the default seed
	// corpus (if it exists).
	exists, err := fileutil.Exists(buildResult.SeedCorpus)
	if err != nil {
		return err
	}
	if exists {
		opts.SeedCorpusDirs = append(opts.SeedCorpusDirs, buildResult.SeedCorpus)
	}

	runnerOpts := &libfuzzer.RunnerOptions{
		Dictionary:         opts.Dictionary,
		EngineArgs:         opts.EngineArgs,
		EnvVars:            []string{"NO_CIFUZZ=1"},
		FuzzTarget:         buildResult.Executable,
		LibraryDirs:        libraryPaths,
		GeneratedCorpusDir: buildResult.GeneratedCorpus,
		KeepColor:          !opts.PrintJSON && !log.PlainStyle(),
		ProjectDir:         opts.ProjectDir,
		ReadOnlyBindings:   []string{buildResult.BuildDir},
		ReportHandler:      reportHandler,
		SeedCorpusDirs:     opts.SeedCorpusDirs,
		Timeout:            opts.Timeout,
		UseMinijail:        opts.UseSandbox,
		Verbose:            viper.GetBool("verbose"),
	}

	// TODO: Only set ReadOnlyBindings if buildResult.BuildDir != ""
	return ExecuteRunner(libfuzzer.NewRunner(runnerOpts))
}

func runJazzer(opts *RunOptions, buildResult *build.BuildResult, reportHandler *reporthandler.ReportHandler) error {
	style := pterm.Style{pterm.Reset, pterm.FgLightBlue}
	log.Infof("Running %s", style.Sprintf(opts.FuzzTest+"::"+opts.TargetMethod))

	// Use user-specified seed corpus dirs (if any) and the default seed
	// corpus (if it exists).
	exists, err := fileutil.Exists(buildResult.SeedCorpus)
	if err != nil {
		return err
	}
	if exists {
		opts.SeedCorpusDirs = append(opts.SeedCorpusDirs, buildResult.SeedCorpus)
	}

	var fuzzerRunner FuzzerRunner

	runnerOpts := &jazzer.RunnerOptions{
		TargetClass:  opts.FuzzTest,
		TargetMethod: opts.TargetMethod,
		ClassPaths:   buildResult.RuntimeDeps,
		LibfuzzerOptions: &libfuzzer.RunnerOptions{
			Dictionary:         opts.Dictionary,
			EngineArgs:         opts.EngineArgs,
			EnvVars:            []string{"NO_CIFUZZ=1"},
			FuzzTarget:         buildResult.Executable,
			GeneratedCorpusDir: buildResult.GeneratedCorpus,
			KeepColor:          !opts.PrintJSON && !log.PlainStyle(),
			ProjectDir:         opts.ProjectDir,
			ReadOnlyBindings:   []string{buildResult.BuildDir},
			ReportHandler:      reportHandler,
			SeedCorpusDirs:     opts.SeedCorpusDirs,
			Timeout:            opts.Timeout,
			UseMinijail:        opts.UseSandbox,
			Verbose:            viper.GetBool("verbose"),
		},
	}

	sourceDirs, err := java.SourceDirs(opts.ProjectDir, opts.BuildSystem)
	if err != nil {
		return err
	}
	testDirs, err := java.TestDirs(opts.ProjectDir, opts.BuildSystem)
	if err != nil {
		return err
	}
	runnerOpts.LibfuzzerOptions.SourceDirs = append(sourceDirs, testDirs...)
	fuzzerRunner = jazzer.NewRunner(runnerOpts)
	return ExecuteRunner(fuzzerRunner)
}

type BuildResultType interface {
	build.BuildResult | build.CBuildResult | build.JavaBuildResult
}

func wrapBuild[BR BuildResultType](opts *RunOptions, build func(*RunOptions) (*BR, error)) (*BR, error) {
	// Note that the build printer should *not* print to c.opts.buildStdout,
	// because that could be a file which is used to store the build log.
	// We don't want the messages of the build printer to be printed to
	// the build log file, so we let it print to stdout or stderr instead.
	var buildPrinterOutput io.Writer
	if opts.PrintJSON {
		buildPrinterOutput = opts.Stdout
	} else {
		buildPrinterOutput = opts.Stderr
	}
	buildPrinter := logging.NewBuildPrinter(buildPrinterOutput, log.BuildInProgressMsg)

	cBuildResult, err := build(opts)
	if err != nil {
		buildPrinter.StopOnError(log.BuildInProgressErrorMsg)
	} else {
		buildPrinter.StopOnSuccess(log.BuildInProgressSuccessMsg, true)
	}
	return cBuildResult, err
}
