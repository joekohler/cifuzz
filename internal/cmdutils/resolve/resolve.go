package resolve

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/util/regexutil"
)

// TODO: use file info of cmake instead of this regex
var cmakeFuzzTestFileNamePattern = regexp.MustCompile(`add_fuzz_test\((?P<fuzzTest>[a-zA-Z0-9_.+=,@~-]+)\s(?P<file>[a-zA-Z0-9_.+=,@~-]+)\)`)

// resolve determines the corresponding fuzz test name to a given source file.
// The path has to be relative to the project directory.
func resolve(path, buildSystem, projectDir string) (string, error) {
	errNoFuzzTest := errors.New("no fuzz test found")

	switch buildSystem {
	case config.BuildSystemCMake:
		cmakeLists, err := findAllCMakeLists(projectDir)
		if err != nil {
			return "", err
		}

		for _, list := range cmakeLists {
			var bs []byte
			bs, err = os.ReadFile(filepath.Join(projectDir, list))
			if err != nil {
				return "", errors.WithStack(err)
			}

			if !strings.Contains(string(bs), "add_fuzz_test") {
				continue
			}

			matches, _ := regexutil.FindAllNamedGroupsMatches(cmakeFuzzTestFileNamePattern, string(bs))
			for _, match := range matches {
				if (filepath.IsAbs(path) && filepath.Join(projectDir, filepath.Dir(list), match["file"]) == path) ||
					filepath.Join(filepath.Dir(list), match["file"]) == path {
					return match["fuzzTest"], nil
				}
			}
		}
		return "", errNoFuzzTest

	case config.BuildSystemBazel:
		var err error
		if filepath.IsAbs(path) {
			path, err = filepath.Rel(projectDir, path)
			if err != nil {
				return "", errors.WithStack(err)
			}
		}

		if runtime.GOOS == "windows" {
			// bazel doesn't allow backslashes in its query
			// but it would be unusual for windows users to
			// use slashes when writing a path so we allow
			// backslashes and replace them internally
			path = strings.ReplaceAll(path, "\\", "/")
		}
		arg := fmt.Sprintf(`attr(generator_function, cc_fuzz_test, same_pkg_direct_rdeps(%q))`, path)
		cmd := exec.Command("bazel", "query", arg)
		out, err := cmd.Output()
		if err != nil {
			// if a bazel query fails it is because no target could be found but it would
			// only return "exit status 7" as error which is no useful information for
			// the user, so instead we return the custom error
			return "", errNoFuzzTest
		}

		fuzzTest := strings.TrimSpace(string(out))
		fuzzTest = strings.TrimSuffix(fuzzTest, "_raw_")

		return fuzzTest, nil

	case config.BuildSystemMaven, config.BuildSystemGradle:
		testDir := filepath.Join(projectDir, "src", "test")
		matches, err := zglob.Glob(filepath.Join(testDir, "**", "*.{java,kt}"))
		if err != nil {
			return "", errors.WithStack(err)
		}

		var pathToFile string
		found := false
		for _, match := range matches {
			if (filepath.IsAbs(path) && match == path) ||
				match == filepath.Join(projectDir, path) {
				pathToFile = match
				found = true
			}

			if !found && runtime.GOOS == "windows" {
				// Try out different slashes under windows to support both formats for user convenience
				// (since zglob.Glob() returns paths with slashes instead of backslashes on windows)
				match = strings.ReplaceAll(match, "/", "\\")
				if (filepath.IsAbs(path) && match == path) ||
					match == filepath.Join(projectDir, path) {
					pathToFile = match
					found = true
				}
			}
		}
		if !found {
			return "", errNoFuzzTest
		}

		fuzzTest, err := cmdutils.ConstructJVMFuzzTestIdentifier(pathToFile, testDir)
		if err != nil {
			return "", err
		}
		return fuzzTest, nil

	default:
		return "", errors.New("The flag '--resolve' only supports the following build systems: CMake, Bazel, Maven, Gradle.")
	}
}

func findAllCMakeLists(projectDir string) ([]string, error) {
	var cmakeLists []string

	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}

		path, err = filepath.Rel(projectDir, path)
		if err != nil {
			return errors.WithStack(err)
		}

		baseName := filepath.Base(path)
		if baseName == "CMakeLists.txt" {
			cmakeLists = append(cmakeLists, path)
		}

		return nil
	})

	return cmakeLists, errors.WithStack(err)
}

func FuzzTestArgument(resolveSourceFile bool, args []string, buildSystem, projectDir string) ([]string, error) {
	if resolveSourceFile {
		fuzzTest, err := resolve(args[0], buildSystem, projectDir)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("Failed to resolve source file %s", args[0]))
		}
		return []string{fuzzTest}, nil
	}

	return args, nil
}
