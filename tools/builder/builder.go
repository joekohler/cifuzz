package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	builderPkg "code-intelligence.com/cifuzz/internal/builder"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
)

func main() {
	flags := pflag.NewFlagSet("builder", pflag.ExitOnError)
	version := flags.StringP("version", "v", "dev", "the target version of cifuzz")
	goos := flags.String("goos", runtime.GOOS, "cross compilation OS, defaults to runtime.GOOS")
	goarch := flags.String("goarch", runtime.GOARCH, "cross compilation GOARCH, defaults to runtime.GOARCH")
	helpRequested := flags.BoolP("help", "h", false, "")
	buildDirFlag := flags.String("build-dir", "cmd/installer/build", "the directory where the build results are written to")
	flags.Bool("verbose", false, "Print verbose output")
	cmdutils.ViperMustBindPFlag("verbose", flags.Lookup("verbose"))
	flags.Bool("coverage", false, "activate coverage instrumentation")
	cmdutils.ViperMustBindPFlag("coverage", flags.Lookup("coverage"))

	if err := flags.Parse(os.Args); err != nil {
		log.Error(errors.WithStack(err))
		os.Exit(1)
	}

	if *helpRequested {
		log.Print("Usage of builder:")
		flags.PrintDefaults()
		os.Exit(0)
	}

	buildDir := *buildDirFlag
	projectDir, err := builderPkg.FindProjectDir()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	if !filepath.IsAbs(buildDir) {
		buildDir = filepath.Join(projectDir, buildDir)
	}

	opts := builderPkg.Options{
		Version:   *version,
		TargetDir: buildDir,
		GOOS:      *goos,
		GOARCH:    *goarch,
		Coverage:  viper.GetBool("coverage"),
	}

	builder, err := builderPkg.NewCIFuzzBuilder(opts)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	err = builder.BuildCIFuzzAndDeps()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
