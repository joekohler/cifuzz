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

	"code-intelligence.com/cifuzz/integration-tests/shared"
	"code-intelligence.com/cifuzz/internal/build"
	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/config"
	"code-intelligence.com/cifuzz/internal/testutil"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/options"
	"code-intelligence.com/cifuzz/util/archiveutil"
	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestAssembleArtifactsJava_Fuzzing(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	projectDir := filepath.Join("testdata", "jazzer", "project")

	fuzzTest := "com.example.FuzzTest"
	anotherFuzzTest := "com.example.AnotherFuzzTest"
	buildDir := filepath.Join(projectDir, "target")

	runtimeDeps := []string{
		// A library in the project's build directory.
		filepath.Join(projectDir, "lib", "mylib.jar"),
		// a directory structure of class files
		filepath.Join(projectDir, "src", "main"),
		filepath.Join(projectDir, "src", "test"),
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
	archiveWriter := archive.NewTarArchiveWriter(bufWriter, true)

	b := newJazzerBundler(&Opts{
		Env:        []string{"FOO=foo"},
		ProjectDir: projectDir,
		tempDir:    tempDir,
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
		"runtime_deps/src/main",
		"runtime_deps/src/test",
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

	out := testutil.MkdirTemp(t, "", "bundler-test-*")
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
	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	testRoot := filepath.Join(tempDir, "src", "test", "java")
	err := os.MkdirAll(testRoot, 0o755)
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

	testDirs := []string{filepath.Join(tempDir, "src", "test")}
	fuzzTests, err := cmdutils.ListJVMFuzzTests(testDirs, "")
	require.NoError(t, err)
	require.ElementsMatchf(t, []string{
		"com.example.FuzzTest::fuzz", "org.example.foo.Bar::fuzzerTestOneInput",
	}, fuzzTests, "Expected to find fuzz test in %s", tempDir)
}

func TestListJVMFuzzTests_DoesNotExist(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	testDirs := []string{filepath.Join(tempDir, "src", "test")}
	fuzzTests, err := cmdutils.ListJVMFuzzTests(testDirs, "")
	require.NoError(t, err)
	require.Empty(t, fuzzTests)
}

func listFilesRecursively(dir string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return errors.WithStack(err)
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.WithStack(err)
		}
		paths = append(paths, relPath)
		return nil
	})
	return paths, errors.Wrapf(err, "Failed to list files from directory %s", dir)
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
		{
			Name:        "com.example.FuzzTest",
			BuildDir:    filepath.Join(projectDir, "target"),
			RuntimeDeps: runtimeDeps,
			ProjectDir:  projectDir,
		},
	}

	bundle, err := os.CreateTemp("", "bundle-archive-")
	require.NoError(t, err)
	bufWriter := bufio.NewWriter(bundle)
	archiveWriter := archive.NewTarArchiveWriter(bufWriter, true)
	t.Cleanup(func() {
		archiveWriter.Close()
		bufWriter.Flush()
		bundle.Close()
	})

	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	b := newJazzerBundler(&Opts{
		tempDir:    tempDir,
		ProjectDir: projectDir,
	}, archiveWriter)

	fuzzers, err := b.assembleArtifacts(buildResults)
	require.NoError(t, err)

	for _, fuzzer := range fuzzers {
		for _, runtimePath := range fuzzer.RuntimePaths {
			assert.NotContains(t, runtimePath, "\\")
		}
	}
}

// Testing a gradle project with two fuzz tests in one class
// and a custom source directory for tests
func TestIntegration_GradleCustomSrcMultipeTests(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// create temp dir for the bundler
	tempDir, err := os.MkdirTemp("", "bundler-temp-dir*")
	require.NoError(t, err)
	t.Cleanup(func() { fileutil.Cleanup(tempDir) })

	// copy test data project to temp dir
	testProject := filepath.Join("testdata", "jazzer", "gradle", "multi-custom")
	projectDir := shared.CopyCustomTestdataDir(t, testProject, "gradle")
	t.Cleanup(func() { fileutil.Cleanup(projectDir) })

	b := newJazzerBundler(&Opts{
		BuildSystem: config.BuildSystemGradle,
		ProjectDir:  projectDir,
		tempDir:     tempDir,
	}, &archive.NullArchiveWriter{})
	fuzzers, err := b.bundle()
	require.NoError(t, err)

	// result should contain two fuzz tests from one class
	assert.Len(t, fuzzers, 2)
	// result should contain fuzz tests with fully qualified names
	assert.Equal(t, "com.example.TestCases::myFuzzTest1", fuzzers[0].Name)
	assert.Equal(t, "com.example.TestCases::myFuzzTest2", fuzzers[1].Name)
}

func TestCreateManifestJar_TargetMethod(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "bundle-temp-dir-")
	jazzerBundler := jazzerBundler{
		opts: &Opts{
			tempDir: tempDir,
		},
	}
	targetClass := "com.example.FuzzTestCase"
	targetMethod := "myFuzzTest"
	jarPath, err := jazzerBundler.createManifestJar(targetClass, targetMethod)
	require.NoError(t, err)

	err = archiveutil.Unzip(jarPath, tempDir)
	require.NoError(t, err)
	manifestPath := filepath.Join(tempDir, "META-INF", "MANIFEST.MF")
	require.FileExists(t, manifestPath)
	content, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), fmt.Sprintf("%s: %s", options.JazzerTargetClassManifest, targetClass))
	assert.Contains(t, string(content), fmt.Sprintf("%s: %s", options.JazzerTargetClassManifestLegacy, targetClass))
	assert.Contains(t, string(content), fmt.Sprintf("%s: %s", options.JazzerTargetMethodManifest, targetMethod))
}

func TestAssembleArtifacts_TargetMethodValidPath(t *testing.T) {
	projectDir := filepath.Join("testdata", "jazzer", "project")
	buildResults := []*build.Result{
		{
			Name:         "com.example.FuzzTest",
			TargetMethod: "myFuzzTest",
		},
	}

	tempDir := testutil.MkdirTemp(t, "", "bundle-*")

	b := newJazzerBundler(&Opts{
		tempDir:    tempDir,
		ProjectDir: projectDir,
	}, &archive.NullArchiveWriter{})

	fuzzers, err := b.assembleArtifacts(buildResults)
	require.NoError(t, err)

	require.Len(t, fuzzers, 1)
	require.Len(t, fuzzers[0].RuntimePaths, 1)
	assert.Contains(t, fuzzers[0].RuntimePaths[0], "com.example.FuzzTest_myFuzzTest")
	assert.Equal(t, fuzzers[0].Name, "com.example.FuzzTest::myFuzzTest")
}

func TestGetAllFuzzTestsAndTargetMethodsForBuild(t *testing.T) {
	opts := &Opts{
		BuildSystem: config.BuildSystemMaven,
		ProjectDir:  filepath.Join("testdata", "jazzer", "maven"),
		FuzzTests:   nil,
	}
	bundler := newJazzerBundler(opts, nil)

	testCases := []struct {
		fuzzTestInBundler     []string
		expectedFuzzTests     []string
		expectedTargetMethods []string
	}{
		{ // No fuzz tests specified
			fuzzTestInBundler: []string{""},
			expectedFuzzTests: []string{
				"com.example.FuzzTestCase1",
				"com.example.FuzzTestCase2",
				"com.example.FuzzTestCase2",
			},
			expectedTargetMethods: []string{
				"myFuzzTest",
				"oneFuzzTest",
				"anotherFuzzTest",
			},
		},
		{ // One class specified that only has one method
			fuzzTestInBundler:     []string{"com.example.FuzzTestCase1"},
			expectedFuzzTests:     []string{"com.example.FuzzTestCase1"},
			expectedTargetMethods: []string{"myFuzzTest"},
		},
		{ // One class specified that has two methods
			fuzzTestInBundler: []string{"com.example.FuzzTestCase2"},
			expectedFuzzTests: []string{
				"com.example.FuzzTestCase2",
				"com.example.FuzzTestCase2"},
			expectedTargetMethods: []string{
				"oneFuzzTest",
				"anotherFuzzTest"},
		},
		{ // One class with target method specified
			fuzzTestInBundler:     []string{"com.example.FuzzTestCase2::anotherFuzzTest"},
			expectedFuzzTests:     []string{"com.example.FuzzTestCase2"},
			expectedTargetMethods: []string{"anotherFuzzTest"},
		},

		{ // Two classes specified, one with target method one without
			fuzzTestInBundler: []string{"" +
				"com.example.FuzzTestCase1",
				"com.example.FuzzTestCase2::anotherFuzzTest"},
			expectedFuzzTests: []string{
				"com.example.FuzzTestCase1",
				"com.example.FuzzTestCase2"},
			expectedTargetMethods: []string{
				"myFuzzTest",
				"anotherFuzzTest"},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase %d", i), func(t *testing.T) {
			bundler.opts.FuzzTests = tc.fuzzTestInBundler
			fuzzTests, targetMethods, err := bundler.fuzzTestIdentifier()
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedFuzzTests, fuzzTests)
			assert.ElementsMatch(t, tc.expectedTargetMethods, targetMethods)
		})
	}
}

func TestGetUniqueArtifactName(t *testing.T) {
	basePath := filepath.Join("testdata", "jazzer", "project", "lib")

	testCases := []struct {
		dependency         string
		uniqueArtifactName string
	}{
		{
			dependency:         filepath.Join(basePath, "mylib.jar"),
			uniqueArtifactName: "mylib.jar",
		},
		{
			dependency:         filepath.Join(basePath, "other", "mylib.jar"),
			uniqueArtifactName: "mylib-1.jar",
		},
		{
			dependency:         filepath.Join(basePath, "testlib.jar"),
			uniqueArtifactName: "testlib.jar",
		},
	}

	artifactsMap := make(map[string]uint)

	for _, tc := range testCases {
		name := getUniqueArtifactName(tc.dependency, artifactsMap)
		assert.Equal(t, tc.uniqueArtifactName, name)
	}
}
