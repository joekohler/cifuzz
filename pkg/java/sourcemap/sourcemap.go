package sourcemap

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/java"
)

// SourceMap provides a mapping from package names
// into the corresponding source file locations
type SourceMap struct {
	JavaPackages map[string][]string `json:"java_packages,omitempty"`
}

func CreateSourceMap(projectDir string, sourceDirs []string) (*SourceMap, error) {
	var sourceFiles []string

	for _, dir := range sourceDirs {
		files, err := zglob.Glob(filepath.Join(dir, "**", "*.{java,kt}"))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		sourceFiles = append(sourceFiles, files...)
	}

	sourceMap := SourceMap{
		JavaPackages: make(map[string][]string),
	}
	for _, sourceFile := range sourceFiles {
		packageName, err := getPackageFromSourceFile(sourceFile)
		if err != nil {
			return nil, err
		}
		if packageName == "" {
			continue
		}
		relPath, err := filepath.Rel(projectDir, sourceFile)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// Replace double slashes on Windows with forward slashes
		relPath = strings.ReplaceAll(relPath, "\\", "/")
		sourceMap.JavaPackages[packageName] = append(sourceMap.JavaPackages[packageName], relPath)
	}

	return &sourceMap, nil
}

func getPackageFromSourceFile(sourceFile string) (string, error) {
	fd, err := os.Open(sourceFile)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer fd.Close()
	return java.GetPackageFromSource(fd), nil
}
