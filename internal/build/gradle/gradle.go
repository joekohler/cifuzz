package gradle

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/runfiles"
	"code-intelligence.com/cifuzz/util/fileutil"
)

var classpathRegex = regexp.MustCompile("(?m)^cifuzz.test.classpath=(?P<classpath>.*)$")
var buildDirRegex = regexp.MustCompile("(?m)^cifuzz.test.buildDir=(?P<buildDir>.*)$")

func FindGradleWrapper(projectDir string) (string, error) {
	wrapper := "gradlew"
	if runtime.GOOS == "windows" {
		wrapper = "gradlew.bat"
	}
	wrapper = filepath.Join(projectDir, wrapper)

	exists, err := fileutil.Exists(wrapper)
	if err != nil {
		return "", errors.WithStack(err)
	}
	if exists {
		return wrapper, nil
	}

	return "", os.ErrNotExist
}

type ParallelOptions struct {
	Enabled bool
	NumJobs uint
}

type BuilderOptions struct {
	ProjectDir string
	Parallel   ParallelOptions
	Stdout     io.Writer
	Stderr     io.Writer
}

func (opts *BuilderOptions) Validate() error {
	// Check that the project dir is set
	if opts.ProjectDir == "" {
		return errors.New("ProjectDir is not set")
	}
	// Check that the project dir exists and can be accessed
	_, err := os.Stat(opts.ProjectDir)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

type Builder struct {
	*BuilderOptions
}

func NewBuilder(opts *BuilderOptions) (*Builder, error) {
	err := opts.Validate()
	if err != nil {
		return nil, err
	}

	b := &Builder{BuilderOptions: opts}

	return b, err
}

func (b *Builder) Build(targetClass string) (*build.Result, error) {
	var flags []string
	if b.Parallel.Enabled {
		flags = append(flags, "--parallel")
	}

	args := append([]string{"testClasses"}, flags...)
	cmd, err := buildGradleCommand(b.ProjectDir, args)

	log.Debugf("Command: %s", cmd.String())
	err = cmd.Run()
	if err != nil {
		// It's expected that gradle might fail due to user configuration,
		// so we print the error without the stack trace.
		err = cmdutils.WrapExecError(errors.WithStack(err), cmd)
		log.Error(err)
		return nil, cmdutils.ErrSilent
	}

	deps, err := b.getDependencies()
	if err != nil {
		return nil, err
	}
	seedCorpus := build.JazzerSeedCorpus(targetClass, b.ProjectDir)
	generatedCorpus := build.JazzerGeneratedCorpus(targetClass, b.ProjectDir)
	result := &build.Result{
		GeneratedCorpus: generatedCorpus,
		SeedCorpus:      seedCorpus,
		RuntimeDeps:     deps,
	}

	return result, nil
}

func (b *Builder) getDependencies() ([]string, error) {
	cmd, err := buildGradleCommand(b.ProjectDir, []string{"printClasspath"})
	if err != nil {
		return nil, err
	}
	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	if err != nil {
		// It's expected that gradle might fail due to user configuration,
		// so we print the error without the stack trace.
		err = cmdutils.WrapExecError(errors.WithStack(err), cmd)
		log.Error(err)
		return nil, cmdutils.ErrSilent
	}
	classpath := classpathRegex.FindStringSubmatch(string(output))
	deps := strings.Split(classpath[1], string(os.PathListSeparator))

	return deps, nil
}

// GetGradleCommand returns the name of the gradle command.
// The gradle wrapper is preferred to use and gradle
// acts as a fallback command.
func GetGradleCommand(projectDir string) (string, error) {
	wrapper, err := FindGradleWrapper(projectDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if wrapper != "" {
		return wrapper, nil
	}

	gradleCmd, err := runfiles.Finder.GradlePath()
	if err != nil {
		return "", errors.WithStack(err)
	}
	return gradleCmd, nil
}

func buildGradleCommand(projectDir string, args []string) (*exec.Cmd, error) {
	gradleCmd, err := GetGradleCommand(projectDir)
	if err != nil {
		return nil, err
	}

	initScript, err := runfiles.Finder.GradleInitScriptPath()
	if err != nil {
		return nil, err
	}
	defaultArgs := []string{"-I", initScript}
	args = append(args, defaultArgs...)

	cmd := exec.Command(gradleCmd, args...)
	cmd.Dir = projectDir

	return cmd, nil
}

func GetBuildDirectory(projectDir string) (string, error) {
	cmd, err := buildGradleCommand(projectDir, []string{"printBuildDir"})
	if err != nil {
		return "", nil
	}

	log.Debugf("Command: %s", cmd.String())
	output, err := cmd.Output()
	result := buildDirRegex.FindStringSubmatch(string(output))
	if result == nil {
		return "", errors.New("Unable to parse gradle build directory from init script.")
	}
	buildDir := result[1]

	return buildDir, nil
}
