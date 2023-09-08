//go:build !windows

package execute

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/cmd/run/adapter"
	"code-intelligence.com/cifuzz/internal/cmd/run/reporthandler"
	"code-intelligence.com/cifuzz/internal/cmdutils"
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
	// Check if we're running as the user specified via the UID environment variable.
	// If not, execute the command as that user.
	uidStr := os.Getenv("CIFUZZ_UID")
	if uidStr == "" {
		return errors.New("CIFUZZ_UID environment variable not set")
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return errors.WithStack(err)
	}
	if uid != os.Getuid() {
		// Change the owner of the current working directory to the specified UID
		// so that the fuzzer can write to the current working directory.
		err = os.Chown(".", uid, -1)
		if err != nil {
			return errors.WithStack(err)
		}

		// Execute the current command as the user specified via the UID environment variable.
		// This is useful when running cifuzz in a container, where the user inside the container
		// may not have the same UID as the user on the host.
		// Note: This is only supported on Linux.
		err = syscall.Setuid(uid)
		if err != nil {
			return errors.WithStack(err)
		}
		path, err := exec.LookPath(os.Args[0])
		if err != nil {
			return errors.WithStack(err)
		}
		log.Infof("Executing command as UID %d: %s", uid, strings.Join(os.Args, " "))
		err = syscall.Exec(path, os.Args, os.Environ())
		if err != nil {
			return errors.WithStack(err)
		}
	}

	fuzzer, err := findFuzzer(c.opts.name, metadata)
	if err != nil {
		return err
	}

	// TODO: create or get real directory for seed corpus
	corpusDirName := "corpus"
	seedDirName := "seed"
	err = os.MkdirAll(seedDirName, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	err = os.MkdirAll(corpusDirName, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	reportHandler, err := reporthandler.NewReportHandler(
		getFuzzerName(fuzzer),
		&reporthandler.ReportHandlerOptions{
			ProjectDir:           fuzzer.ProjectDir,
			PrintJSON:            c.opts.PrintJSON,
			ManagedSeedCorpusDir: seedDirName,
		})
	if err != nil {
		return err
	}

	runnerOpts := &libfuzzer.RunnerOptions{
		FuzzTarget:         fuzzer.Path,
		ProjectDir:         fuzzer.ProjectDir,
		UseMinijail:        false,
		LibraryDirs:        fuzzer.LibraryPaths,
		Verbose:            viper.GetBool("verbose"),
		ReportHandler:      reportHandler,
		GeneratedCorpusDir: corpusDirName,
		EnvVars:            []string{"NO_CIFUZZ=1"},
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
