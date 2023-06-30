package cmdutils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattn/go-zglob"
	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/util/regexutil"
)

func ListNodeFuzzTests(projectDir string, prefixFilter string) ([]string, error) {
	// use zglob to support globbing in windows
	fuzzTestFiles, err := zglob.Glob(filepath.Join(projectDir, "**", "*.fuzz.*"))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var fuzzTests []string
	for _, testFile := range fuzzTestFiles {
		methods, err := getTargetMethodsFromNodeTestFile(testFile)
		if err != nil {
			return nil, err
		}

		fuzzTest := filepath.Base(testFile)
		if strings.HasSuffix(fuzzTest, ".ts") {
			fuzzTest = strings.TrimSuffix(fuzzTest, ".fuzz.ts")
		} else {
			fuzzTest = strings.TrimSuffix(fuzzTest, ".fuzz.js")
		}
		if len(methods) == 1 {
			if prefixFilter == "" || strings.HasPrefix(testFile, prefixFilter) {
				fuzzTests = append(fuzzTests, fuzzTest)
			}
			continue
		}

		for _, method := range methods {
			fuzzTestIdentifier := fuzzTest + ":" + fmt.Sprintf("%q", method)
			if prefixFilter == "" || strings.HasPrefix(fuzzTestIdentifier, prefixFilter) {
				fuzzTests = append(fuzzTests, fuzzTestIdentifier)
			}
		}
	}

	return fuzzTests, nil
}

func getTargetMethodsFromNodeTestFile(path string) ([]string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var targetMethods []string
	fuzzTestRegex := regexp.MustCompile(`\.fuzz\s*\(\s*"(?P<fuzzTest>(\w|\s|\d)+)`)
	matches, _ := regexutil.FindAllNamedGroupsMatches(fuzzTestRegex, string(bytes))
	for _, match := range matches {
		targetMethods = append(targetMethods, match["fuzzTest"])
	}

	return targetMethods, nil
}
