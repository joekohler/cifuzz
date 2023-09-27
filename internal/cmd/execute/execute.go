//go:build !windows

package execute

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/cmd/run/adapter"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/container"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runner/jazzer"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
	"code-intelligence.com/cifuzz/util/fileutil"
	"code-intelligence.com/cifuzz/util/stringutil"
)

type executeOpts struct {
	PrintJSON           bool `mapstructure:"print-json"`
	SingleFuzzTest      bool `mapstructure:"single-fuzz-test"`
	PrintBundleMetadata bool `mapstructure:"print-bundle-metadata"`

	name string
}

type executeCmd struct {
	*cobra.Command
	opts *executeOpts
}

func New() *cobra.Command {
	var bindFlags func()

	opts := &executeOpts{}
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute a fuzz test bundle locally (experimental)",
		Long: `This command executes a cifuzz fuzz test bundle locally.
It can be used as an experimental alternative to cifuzz_runner.
It is currently only intended for use with the 'cifuzz container' subcommand.

`,
		Example: "cifuzz execute [fuzz test]",
		Args:    cobra.MaximumNArgs(1),
		PreRun: func(cmd *cobra.Command, args []string) {
			// Bind viper keys to flags. We can't do this in the New
			// function, because that would re-bind viper keys which
			// were bound to the flags of other commands before.
			bindFlags()
			cmdutils.ViperMustBindPFlag("single-fuzz-test", cmd.Flags().Lookup("single-fuzz-test"))
			cmdutils.ViperMustBindPFlag("print-bundle-metadata", cmd.Flags().Lookup("print-bundle-metadata"))
			cmdutils.ViperMustBindPFlag("stop-signal-file", cmd.Flags().Lookup("stop-signal-file"))
			opts.SingleFuzzTest = viper.GetBool("single-fuzz-test")
			opts.PrintBundleMetadata = viper.GetBool("print-bundle-metadata")
			opts.PrintJSON = viper.GetBool("print-json")
		},
		RunE: func(c *cobra.Command, args []string) error {
			if signalFile := viper.GetString("stop-signal-file"); signalFile != "" {
				defer func() {
					_, err := os.Create(signalFile)
					if err != nil {
						log.Errorf(err, "Failed to create stop signal file: %v", err)
					}
				}()
			}

			metadata, err := getMetadata()
			if err != nil {
				return err
			}

			if opts.PrintBundleMetadata {
				metadataJSON, err := stringutil.ToJSONString(metadata)
				if err != nil {
					return err
				}
				fmt.Println(metadataJSON)
			}

			// If there are no arguments provided, provide a helpful message and list all available fuzzers.
			if len(args) == 0 && !opts.SingleFuzzTest {
				return printNotice(metadata)
			}

			if opts.SingleFuzzTest && len(args) > 0 {
				msg := "The <fuzz test> argument cannot be used with the --single-fuzz-test flag."
				return cmdutils.WrapIncorrectUsageError(errors.New(msg))
			}

			if !opts.SingleFuzzTest {
				opts.name = args[0]
			}

			cmd := executeCmd{Command: c, opts: opts}
			return cmd.run(metadata)
		},
	}

	cmdutils.DisableConfigCheck(cmd)

	cmd.Flags().Bool("single-fuzz-test", false, "Run the only fuzz test in the bundle (without specifying the fuzz test name).")
	cmd.Flags().Bool("print-bundle-metadata", false, "Print the bundle metadata as JSON.")
	cmd.Flags().String("stop-signal-file", "", "CI Fuzz will create a file 'cifuzz-execution-finished' upon exit")

	// Note: If a flag should be configurable via viper as well (i.e.
	//       via cifuzz.yaml and CIFUZZ_* environment variables), bind
	//       it to viper in the PreRun function.
	bindFlags = cmdutils.AddFlags(cmd,
		cmdutils.AddPrintJSONFlag,
	)

	return cmd
}

func (c *executeCmd) run(metadata *archive.Metadata) error {
	fuzzer, err := findFuzzer(c.opts.name, metadata)
	if err != nil {
		return err
	}

	err = os.MkdirAll(container.ManagedSeedCorpusDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	err = os.MkdirAll(container.GeneratedCorpusDir, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	printerOutput := os.Stdout
	jsonOutput := io.Discard
	if c.opts.PrintJSON {
		printerOutput = os.Stderr
		jsonOutput = os.Stdout
	}

	reportHandler, err := reporthandler.NewReportHandler(
		getFuzzerName(fuzzer),
		&reporthandler.ReportHandlerOptions{
			ProjectDir:           fuzzer.ProjectDir,
			ManagedSeedCorpusDir: container.ManagedSeedCorpusDir,
			// Saving findings is currently broken when the container is run
			// as a non-root user and the build system is bazel. This is a
			// quick workaround to avoid breaking the container when it's run
			// in that configuration.
			SkipSavingFinding: true,
			PrinterOutput:     printerOutput,
			JSONOutput:        jsonOutput,
		})
	if err != nil {
		return err
	}

	runnerOpts := &libfuzzer.RunnerOptions{
		FuzzTarget:         fuzzer.Path,
		EngineArgs:         fuzzer.EngineOptions.Flags,
		Timeout:            time.Duration(fuzzer.MaxRunTime) * time.Second,
		ProjectDir:         fuzzer.ProjectDir,
		UseMinijail:        false,
		LibraryDirs:        fuzzer.LibraryPaths,
		Verbose:            viper.GetBool("verbose"),
		ReportHandler:      reportHandler,
		GeneratedCorpusDir: container.GeneratedCorpusDir,
		EnvVars:            []string{"NO_CIFUZZ=1"},
		KeepColor:          !c.opts.PrintJSON && !log.PlainStyle(),
	}

	// Specify the dictionary file if the bundle includes one.
	dictFileName := "dict"
	exists, err := fileutil.Exists(dictFileName)
	if err != nil {
		return err
	}
	if exists {
		runnerOpts.Dictionary = dictFileName
	}

	// Specify user-supplied seed corpus dirs if the bundle includes any.
	userSeedCorpusDir := "seeds"
	entries, err := os.ReadDir(userSeedCorpusDir)
	// Don't return an error if the directory doesn't exist.
	if err != nil && !os.IsNotExist(err) {
		return errors.WithStack(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return errors.Errorf("unexpected file in user seed corpus dir %q: %s", userSeedCorpusDir, entry.Name())
		}
		seedCorpusDir := fmt.Sprintf("%s/%s", userSeedCorpusDir, entry.Name())
		runnerOpts.SeedCorpusDirs = append(runnerOpts.SeedCorpusDirs, seedCorpusDir)
	}

	var runner adapter.FuzzerRunner

	switch fuzzer.Engine {
	case "JAVA_LIBFUZZER":

		name := fuzzer.Name
		method := ""
		if strings.Contains(fuzzer.Name, "::") {
			split := strings.Split(fuzzer.Name, "::")
			name = split[0]
			method = split[1]
		}
		runnerOpts := &jazzer.RunnerOptions{
			TargetClass:      name,
			TargetMethod:     method,
			ClassPaths:       fuzzer.RuntimePaths,
			LibfuzzerOptions: runnerOpts,
		}
		runner = jazzer.NewRunner(runnerOpts)
	default:
		runner = libfuzzer.NewRunner(runnerOpts)
	}

	return adapter.ExecuteFuzzerRunner(runner)
}

// getMetadata returns the bundle metadata from the bundle.yaml file.
func getMetadata() (*archive.Metadata, error) {
	exists, err := fileutil.Exists(archive.MetadataFileName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.Errorf("bundle metadata file '%s' does not exist. Execute command should be run in a folder with an unpacked cifuzz bundle.", archive.MetadataFileName)
	}

	metadataBytes, err := os.ReadFile(archive.MetadataFileName)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	metadata := &archive.Metadata{}
	err = metadata.FromYaml(metadataBytes)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func printNotice(metadata *archive.Metadata) error {
	_ = pterm.DefaultBigText.WithLetters(
		putils.LettersFromStringWithStyle("Fuzz", pterm.FgCyan.ToStyle()),
		putils.LettersFromString(" "),
		putils.LettersFromStringWithStyle("Container", pterm.FgLightMagenta.ToStyle())).
		Render()

	fmt.Println("")
	fmt.Printf("This container is based on: %s\n", metadata.RunEnvironment.Docker)
	fmt.Println("")

	fmt.Printf("Available fuzzers:\n")
	for _, fuzzer := range metadata.Fuzzers {
		fuzzerName := fuzzer.Name
		if fuzzerName == "" {
			fuzzerName = fuzzer.Name
		}
		fmt.Printf("  %s\n", fuzzerName)
		fmt.Printf("    using: %s\n", fuzzer.Engine)
		fmt.Printf("    run fuzz test with: cifuzz execute %s\n", fuzzerName)
		fmt.Println("")
	}
	return nil
}

// getFuzzerName returns the fuzzer name. Some Fuzzer define Name (jazzer) and some define Target (libfuzzer).
func getFuzzerName(fuzzer *archive.Fuzzer) string {
	if fuzzer.Name != "" {
		return fuzzer.Name
	}
	return fuzzer.Target
}

// findFuzzer returns the fuzzer with the given name in Fuzzers list in Bundle Metadata.
func findFuzzer(nameToFind string, bundleMetadata *archive.Metadata) (*archive.Fuzzer, error) {
	// libFuzzer fuzz tests contain two entries in the metadata file, one
	// for fuzzing and one for coverage. We want the fuzzing entries, which
	// are listed first.
	fuzzers := make(map[string]*archive.Fuzzer)
	for _, fuzzer := range bundleMetadata.Fuzzers {
		name := getFuzzerName(fuzzer)
		if _, ok := fuzzers[name]; !ok {
			fuzzers[name] = fuzzer
		}
	}

	if nameToFind == "" {
		// Check if there is only one fuzzer in the bundle.
		if len(fuzzers) == 1 {
			// Return the only fuzzer in the bundle.
			for _, fuzzer := range fuzzers {
				return fuzzer, nil
			}
		}
		return nil, errors.Errorf("no fuzzer name provided and more than one fuzzer found in a bundle metadata file")
	}

	if fuzzer, ok := fuzzers[nameToFind]; ok {
		// TODO: is there a more validation we want to perform? If so, should it be part of the metadata parsing?
		// TODO: is multiple matches a valid scenario?
		return fuzzer, nil
	}

	return nil, errors.Errorf("fuzzer '%s' not found in a bundle metadata file", nameToFind)
}
