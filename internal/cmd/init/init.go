package init

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/dependencies"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
	"code-intelligence.com/cifuzz/util/fileutil"
)

const (
	GradleMultiProjectWarningMsg = "For multi-project builds, you should setup cifuzz in the subprojects containing the fuzz tests."
)

type Options struct {
	Dir         string
	BuildSystem string
}

func New() *cobra.Command {
	return NewWithOptions(&Options{})
}

func NewWithOptions(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set up a project for use with cifuzz",
		Long: `This command sets up a project for use with cifuzz, creating a
'cifuzz.yaml' config file.`,
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			if opts.Dir == "" {
				opts.Dir, err = os.Getwd()
				if err != nil {
					return errors.WithStack(err)
				}
			}

			opts.BuildSystem, err = config.DetermineBuildSystem(opts.Dir)
			if err != nil {
				log.Error(err)
				return cmdutils.WrapSilentError(err)
			}

			err = config.ValidateBuildSystem(opts.BuildSystem)
			if err != nil {
				log.Error(err)
				return cmdutils.WrapSilentError(err)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(opts)
		},
	}

	cmdutils.DisableConfigCheck(cmd)

	return cmd
}

func run(opts *Options) error {
	setUpAndMentionBuildSystemIntegrations(opts.Dir, opts.BuildSystem)

	log.Debugf("Creating config file in directory: %s", opts.Dir)
	configpath, err := config.CreateProjectConfig(opts.Dir)
	if err != nil {
		// explicitly inform the user about an existing config file
		if errors.Is(err, os.ErrExist) && configpath != "" {
			log.Warnf("Config already exists in %s", configpath)
			err = cmdutils.ErrSilent
		}
		log.Error(err, "Failed to create config")
		return err
	}
	log.Successf("Configuration saved in %s", fileutil.PrettifyPath(configpath))

	log.Print(`
Use 'cifuzz create' to create your first fuzz test.`)
	return nil
}

func setUpAndMentionBuildSystemIntegrations(dir string, buildSystem string) {
	switch buildSystem {
	case config.BuildSystemBazel:
		log.Print(fmt.Sprintf(messaging.Instructions(buildSystem), dependencies.RulesFuzzingHTTPArchiveRule, dependencies.CIFuzzBazelCommit))
	case config.BuildSystemCMake:
		// Note: We set NO_SYSTEM_ENVIRONMENT_PATH to avoid that the
		// system-wide cmake package takes precedence over a package
		// from a per-user installation (which is what we want, per-user
		// installations should usually take precedence over system-wide
		// installations).
		//
		// The find_package search procedure is described in
		// https://cmake.org/cmake/help/latest/command/find_package.html#config-mode-search-procedure.
		//
		// Without NO_SYSTEM_ENVIRONMENT_PATH, find_package looks in
		// paths with prefixes from the PATH environment variable in
		// step 5 (omitting any trailing "/bin").
		// The PATH usually includes "/usr/local/bin", which means that
		// find_package searches in "/usr/local/share/cifuzz" in this
		// step, which is the path we use for a system-wide installation.
		//
		// The per-user directory is searched in step 6.
		//
		// With NO_SYSTEM_ENVIRONMENT_PATH, the system-wide installation
		// directory is only searched in step 7.
		log.Print(messaging.Instructions(buildSystem))
	case config.BuildSystemNodeJS:
		if os.Getenv("CIFUZZ_PRERELEASE") != "" {
			log.Print(messaging.Instructions(buildSystem))
		} else {
			log.Print("cifuzz does not support NodeJS projects yet.")
			os.Exit(1)
		}
	case config.BuildSystemMaven:
		log.Print(messaging.Instructions(buildSystem))
	case config.BuildSystemGradle:
		gradleBuildLanguage, err := config.DetermineGradleBuildLanguage(dir)
		if err != nil {
			log.Debug(err)
			return
		}

		isGradleMultiProject, err := config.IsGradleMultiProject(dir)
		if err != nil {
			log.Debug(err)
			return
		}
		if isGradleMultiProject {
			log.Warn(GradleMultiProjectWarningMsg)
		}

		log.Print(messaging.Instructions(string(gradleBuildLanguage)))
	}
}
