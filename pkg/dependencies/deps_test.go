package dependencies

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/pkg/mocks"
)

func TestCheck(t *testing.T) {
	keys := []Key{CMake}
	deps := getDeps(keys)

	dep := deps[CMake]
	dep.GetVersion = func(d *Dependency, _ string) (*semver.Version, error) {
		return &d.MinVersion, nil
	}

	finder := &mocks.RunfilesFinderMock{}
	finder.On("CMakePath").Return("cmake", nil)

	err := check(keys, deps, finder, "")
	require.NoError(t, err)
}

func TestCheck_NotInstalled(t *testing.T) {
	keys := []Key{CMake}
	deps := getDeps(keys)

	finder := &mocks.RunfilesFinderMock{}
	finder.On("CMakePath").Return("", errors.New("missing-error"))

	err := check(keys, deps, finder, "")
	require.Error(t, err)
}

func TestCheck_WrongVersion(t *testing.T) {
	keys := []Key{CMake}
	deps := getDeps(keys)

	// overwrite GetVersion for clang
	dep := deps[CMake]
	dep.GetVersion = func(d *Dependency, _ string) (*semver.Version, error) {
		return semver.MustParse("1.0.0"), nil
	}

	finder := &mocks.RunfilesFinderMock{}
	finder.On("CMakePath").Return("cmake", nil)

	err := check(keys, deps, finder, "")
	require.Error(t, err)
}

func TestCheck_ShortVersion(t *testing.T) {
	keys := []Key{CMake}
	deps := getDeps(keys)

	// overwrite GetVersion for clang
	dep := deps[CMake]
	dep.GetVersion = func(d *Dependency, _ string) (*semver.Version, error) {
		return semver.MustParse("3.16"), nil
	}

	finder := &mocks.RunfilesFinderMock{}
	finder.On("CMakePath").Return("cmake", nil)

	err := check(keys, deps, finder, "")
	require.NoError(t, err)
}

func TestCheck_UnableToGetVersion(t *testing.T) {
	keys := []Key{CMake}
	deps := getDeps(keys)

	// overwrite GetVersion for clang
	dep := deps[CMake]
	dep.GetVersion = func(d *Dependency, _ string) (*semver.Version, error) {
		return nil, errors.New("version-error")
	}

	finder := &mocks.RunfilesFinderMock{}
	finder.On("CMakePath").Return("cmake", nil)

	err := check(keys, deps, finder, "")
	require.NoError(t, err)
}

// TestCheck_SpecialCaseGradle tests if the special case of gradle is handled
// correctly. The preferred way of using gradle is using the 'gradlew' (wrapper)
// in the project dir and this should also work without needing gradle to be
// installed at all. If there is no gradlew or an empty project dir is given
// it should default back to needing gradle installed.
func TestCheck_SpecialCaseGradle(t *testing.T) {
	keys := []Key{Gradle}
	deps := getDeps(keys)

	// no gradle and no gradlew
	finder := &mocks.RunfilesFinderMock{}
	finder.On("GradlePath").Return("", errors.New("missing-error"))
	err := check(keys, deps, finder, "")
	require.Error(t, err)

	// gradle but no gradlew
	finder = &mocks.RunfilesFinderMock{}
	finder.On("GradlePath").Return("gradle", nil)
	err = check(keys, deps, finder, "")
	require.NoError(t, err)

	// no gradle but gradlew in project dir
	finder = &mocks.RunfilesFinderMock{}
	err = check(keys, deps, finder, filepath.Join("testdata", "gradle"))
	require.NoError(t, err)
}
