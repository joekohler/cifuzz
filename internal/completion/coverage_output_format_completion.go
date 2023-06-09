package completion

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/coverage"
	"code-intelligence.com/cifuzz/pkg/log"
)

// ValidCoverageOutputFormat can be used as a cobra ValidArgsFunction
// that completes the --format flag of the cifuzz coverage command.
func ValidCoverageOutputFormat(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Change the directory if the `--directory` flag was set
	err := cmdutils.Chdir()
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	// Read the project config to figure out the build system
	conf := struct {
		BuildSystem string `mapstructure:"build-system"`
		ProjectDir  string `mapstructure:"project-dir"`
	}{}
	err = config.FindAndParseProjectConfig(&conf)
	if err != nil {
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	outputFormat, ok := coverage.ValidOutputFormats[conf.BuildSystem]
	if !ok {
		err := errors.Errorf("Unknown build system %q", conf.BuildSystem)
		log.Error(err)
		return nil, cobra.ShellCompDirectiveError
	}

	return outputFormat, cobra.ShellCompDirectiveNoFileComp
}
