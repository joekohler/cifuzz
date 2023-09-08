package runner

import (
	"strings"

	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/build/java/maven"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
)

type MavenRunner struct {
}

func (r *MavenRunner) CheckDependencies(projectDir string) error {
	return dependencies.Check([]dependencies.Key{
		dependencies.Java,
		dependencies.Maven,
	}, projectDir)
}

func (r *MavenRunner) Run(opts *RunOptions) (*reporthandler.ReportHandler, error) {

	buildResult, err := wrapBuild[build.BuildResult](opts, r.build)
	if err != nil {
		return nil, err
	}

	if opts.BuildOnly {
		return nil, nil
	}

	err = prepareCorpusDir(opts, buildResult)
	if err != nil {
		return nil, err
	}

	reportHandler, err := createReportHandler(opts, buildResult)
	if err != nil {
		return nil, err
	}

	err = runJazzer(opts, buildResult, reportHandler)
	if err != nil {
		return nil, err
	}

	return reportHandler, nil
}

func (r *MavenRunner) build(opts *RunOptions) (*build.BuildResult, error) {

	if len(opts.ArgsToPass) > 0 {
		log.Warnf("Passing additional arguments is not supported for Maven.\n"+
			"These arguments are ignored: %s", strings.Join(opts.ArgsToPass, " "))
	}

	var builder *maven.Builder
	builder, err := maven.NewBuilder(&maven.BuilderOptions{
		ProjectDir: opts.ProjectDir,
		Parallel: maven.ParallelOptions{
			Enabled: viper.IsSet("build-jobs"),
			NumJobs: opts.NumBuildJobs,
		},
		Stdout: opts.BuildStdout,
		Stderr: opts.BuildStderr,
	})
	if err != nil {
		return nil, err
	}

	var buildResult *build.BuildResult
	buildResult, err = builder.Build()
	if err != nil {
		return nil, err
	}
	return buildResult, err
}
