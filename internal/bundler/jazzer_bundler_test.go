package bundler

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestAssembleArtifactsJava_Fuzzing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bundle-*")
	require.NoError(t, err)
	defer fileutil.Cleanup(tempDir)
	require.NoError(t, err)

	projectDir := filepath.Join("testdata", "jazzer", "project")

	fuzzTest := "com.example.FuzzTest"
	anotherFuzzTest := "com.example.AnotherFuzzTest"
	buildDir := filepath.Join(projectDir, "target")

	runtimeDeps := []string{
		// A library in the project's build directory.
		filepath.Join(projectDir, "lib", "mylib.jar"),
		// a directory structure of class files
		filepath.Join(projectDir, "classes"),
		filepath.Join(projectDir, "test-classes"),
	}

	buildResults := []*build.Result{}
	buildResult := &build.Result{
		Name:        fuzzTest,
		BuildDir:    buildDir,
		RuntimeDeps: runtimeDeps,
		ProjectDir:  projectDir,
	}
	anotherBuildResult := &build.Result{
		Name:        anotherFuzzTest,
		BuildDir:    buildDir,
		RuntimeDeps: runtimeDeps,
		ProjectDir:  projectDir,
	}
	buildResults = append(buildResults, buildResult, anotherBuildResult)

	bundle, err := os.CreateTemp("", "bundle-archive-")
	require.NoError(t, err)
	bufWriter := bufio.NewWriter(bundle)
	archiveWriter := archive.NewArchiveWriter(bufWriter)

	b := newJazzerBundler(&Opts{
		Env:     []string{"FOO=foo"},
		tempDir: tempDir,
	}, archiveWriter)
	fuzzers, err := b.assembleArtifacts(buildResults)
	require.NoError(t, err)

	err = archiveWriter.Close()
	require.NoError(t, err)
	err = bufWriter.Flush()
	require.NoError(t, err)
	err = bundle.Close()
	require.NoError(t, err)

	// we expect forward slashes even on windows, see also:
	// TestAssembleArtifactsJava_WindowsForwardSlashes
	expectedDeps := []string{
		// manifest.jar should always be first element in runtime paths
		fmt.Sprintf("%s/manifest.jar", fuzzTest),
		"runtime_deps/mylib.jar",
		"runtime_deps/classes",
		"runtime_deps/test-classes",
	}
	expectedFuzzer := &archive.Fuzzer{
		Name:         buildResult.Name,
		Engine:       "JAVA_LIBFUZZER",
		ProjectDir:   buildResult.ProjectDir,
		RuntimePaths: expectedDeps,
		EngineOptions: archive.EngineOptions{
			Env:   b.opts.Env,
			Flags: b.opts.EngineArgs,
		},
	}
	require.Equal(t, 2, len(fuzzers))
	require.Equal(t, *expectedFuzzer, *fuzzers[0])

	// Unpack archive contents with tar.
	out, err := os.MkdirTemp("", "bundler-test-*")
	require.NoError(t, err)
	cmd := exec.Command("tar", "-xvf", bundle.Name(), "-C", out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("Command: %v", cmd.String())
	err = cmd.Run()
	require.NoError(t, err)

	// Check that the archive has the expected contents
	expectedContents, err := listFilesRecursively(filepath.Join("testdata", "jazzer", "expected-archive-contents"))
	require.NoError(t, err)
	actualContents, err := listFilesRecursively(out)
	require.NoError(t, err)
	require.Equal(t, expectedContents, actualContents)
}

func TestListFuzzTests(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bundle-*")
	require.NoError(t, err)
	defer fileutil.Cleanup(tempDir)
	require.NoError(t, err)

	testRoot := filepath.Join(tempDir, "src", "test", "java")
	err = os.MkdirAll(testRoot, 0o755)
	require.NoError(t, err)
	firstPackage := filepath.Join(testRoot, "com", "example")
	err = os.MkdirAll(firstPackage, 0o755)
	require.NoError(t, err)
	secondPackage := filepath.Join(testRoot, "org", "example", "foo")
	err = os.MkdirAll(secondPackage, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(firstPackage, "FuzzTest.java"), []byte(`
package com.example;

import com.code_intelligence.jazzer.junit.FuzzTest;

class FuzzTest {
    @FuzzTest
    void fuzz(byte[] data) {}
}
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(secondPackage, "Bar.java"), []byte(`
package org.example.foo;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class Bar {
    public static void fuzzerTestOneInput(FuzzedDataProvider data) {}
}
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(secondPackage, "Baz.txt"), []byte(`
package org.example.foo;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class Baz {
    public static void fuzzerTestOneInput(FuzzedDataProvider data) {}
}
`), 0o644)
	require.NoError(t, err)

	fuzzTests, err := cmdutils.ListJVMFuzzTests(tempDir)
	require.NoError(t, err)
	require.ElementsMatchf(t, []string{
		"com.example.FuzzTest", "org.example.foo.Bar",
	}, fuzzTests, "Expected to find fuzz test in %s", tempDir)
}

func TestListJVMFuzzTests_DoesNotExist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bundle-*")
	require.NoError(t, err)
	defer fileutil.Cleanup(tempDir)
	require.NoError(t, err)

	fuzzTests, err := cmdutils.ListJVMFuzzTests(tempDir)
	require.NoError(t, err)
	require.Empty(t, fuzzTests)
}

func listFilesRecursively(dir string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.WithStack(err)
		}
		paths = append(paths, relPath)
		return nil
	})
	return paths, err
}

// As long as we only have linux based runner we should make sure
// that the runtime paths are using forward slashes even if the
// bundle was created on windows
func TestAssembleArtifactsJava_WindowsForwardSlashes(t *testing.T) {
	projectDir := filepath.Join("testdata", "jazzer", "project")
	runtimeDeps := []string{
		filepath.Join(projectDir, "lib", "mylib.jar"),
	}

	buildResults := []*build.Result{
		&build.Result{
			Name:        "com.example.FuzzTest",
			BuildDir:    filepath.Join(projectDir, "target"),
			RuntimeDeps: runtimeDeps,
			ProjectDir:  projectDir,
		},
	}

	bundle, err := os.CreateTemp("", "bundle-archive-")
	require.NoError(t, err)
	bufWriter := bufio.NewWriter(bundle)
	archiveWriter := archive.NewArchiveWriter(bufWriter)
	t.Cleanup(func() {
		archiveWriter.Close()
		bufWriter.Flush()
		bundle.Close()
	})

	tempDir, err := os.MkdirTemp("", "bundle-*")
	require.NoError(t, err)
	t.Cleanup(func() { fileutil.Cleanup(tempDir) })

	b := newJazzerBundler(&Opts{
		tempDir: tempDir,
	}, archiveWriter)

	fuzzers, err := b.assembleArtifacts(buildResults)
	require.NoError(t, err)

	for _, fuzzer := range fuzzers {
		for _, runtimePath := range fuzzer.RuntimePaths {
			assert.NotContains(t, runtimePath, "\\")
		}
	}
}
