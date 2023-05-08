package dependencies

import (
	"errors"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/pkg/mocks"
)

func TestCheck(t *testing.T) {
	keys := []Key{CMake}
	deps := getDeps(keys)

	dep := deps[CMake]
	dep.GetVersion = func(d *Dependency) (*semver.Version, error) {
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
	dep.GetVersion = func(d *Dependency) (*semver.Version, error) {
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
	dep.GetVersion = func(d *Dependency) (*semver.Version, error) {
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
	dep.GetVersion = func(d *Dependency) (*semver.Version, error) {
		return nil, errors.New("version-error")
	}

	finder := &mocks.RunfilesFinderMock{}
	finder.On("CMakePath").Return("cmake", nil)

	err := check(keys, deps, finder, "")
	require.NoError(t, err)
}
