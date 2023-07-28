package stubs

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

//go:embed fuzz-test.cpp.tmpl
var cppStub []byte

//go:embed fuzzTest.java.tmpl
var javaStub []byte

//go:embed fuzzTest.ktl.tmpl
var kotlinStub []byte

//go:embed test.fuzz.js.tmpl
var javaScriptStub []byte

//go:embed test.fuzz.ts.tmpl
var typeScriptStub []byte

// Create creates a stub based for the given test type
func Create(path string, testType config.FuzzTestType) error {
	exists, err := fileutil.Exists(path)
	if err != nil {
		return err
	}
	if exists {
		return errors.WithStack(os.ErrExist)
	}

	// read matching template
	var content []byte
	switch testType {
	case config.CPP:
		content = cppStub
	case config.Java, config.Kotlin:
		{
			stub := string(javaStub)
			if testType == config.Kotlin {
				stub = string(kotlinStub)
			}
			fileNameExtension, found := config.TestTypeFileNameExtension(testType)
			if !found {
				return errors.WithStack(err)
			}

			baseName := strings.TrimSuffix(filepath.Base(path), fileNameExtension)
			content = []byte(strings.Replace(stub, "__CLASS_NAME__", baseName, 1))

			// If we have a valid package name we add it to the template
			// We assume the project has the standard java project structure
			if filepath.Dir(path) != "" {
				packagePath := strings.TrimPrefix(filepath.Dir(path),
					filepath.Join("src", "test", string(testType))+string(os.PathSeparator))
				packagePath = strings.ReplaceAll(packagePath, string(os.PathSeparator), ".")

				packageName := fmt.Sprintf("package %s;", packagePath)
				if testType == config.Kotlin {
					packageName = strings.TrimSuffix(packageName, ";")
				}
				content = []byte(strings.Replace(string(content), "__PACKAGE__", packageName, 1))
			}
		}
	case config.JavaScript:
		content = javaScriptStub
	case config.TypeScript:
		content = typeScriptStub
	}

	if testType == config.Rust {
		cmd := exec.Command("cargo", "fuzz", "add", filepath.Base(path))
		log.Debugf("Working directory: %s", cmd.Dir)
		log.Debugf("Command: %s", cmd.String())
		err := cmd.Run()
		if err != nil {
			return errors.WithStack(err)
		}

		return nil
	}

	// write stub
	if content != nil && path != "" {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// FuzzTestFilename returns a proposal for a filename,
// depending on the test type and given directory.
// The filename should follow the conventions of the type.
func FuzzTestFilename(testType config.FuzzTestType) (string, error) {
	var filePattern, basename, ext, filename string

	switch testType {
	case config.CPP:
		basename = "my_fuzz_test"
		ext = "cpp"
		filePattern = "%s_%d.%s"
	case config.Kotlin:
		basename = "MyClassFuzzTest"
		ext = "kt"
		filePattern = "%s%d.%s"
	case config.Java:
		basename = "MyClassFuzzTest"
		ext = "java"
		filePattern = "%s%d.%s"
	case config.JavaScript:
		basename = "myTest"
		ext = "fuzz.js"
		filePattern = "%s%d.%s"
	case config.TypeScript:
		basename = "myTest"
		ext = "fuzz.ts"
		filePattern = "%s%d.%s"
	case config.Rust:
		basename = "fuzz_target"
		ext = "rs"
		filePattern = "%s_%d.%s"

	default:
		return "", errors.New("unable to suggest filename: unknown test type")
	}

	for counter := 1; ; counter++ {
		baseDir := "."
		if testType == config.Rust {
			baseDir = filepath.Join("fuzz", "fuzz_targets")
		}
		filename = filepath.Join(baseDir, fmt.Sprintf(filePattern, basename, counter, ext))
		exists, err := fileutil.Exists(filename)
		if err != nil {
			return "", err
		}

		if !exists {
			return filename, nil
		}
	}
}
