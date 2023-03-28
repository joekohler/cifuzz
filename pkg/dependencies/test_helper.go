package dependencies

import (
	"fmt"
	"testing"

	"github.com/Masterminds/semver"
)

func GetDep(key Key) *Dependency {
	dep, found := deps[key]
	if found {
		return dep
	}
	panic(fmt.Sprintf("Unknown dependency %s", key))
}

func getDeps(keys []Key) Dependencies {
	deps := Dependencies{}
	for _, key := range keys {
		deps[key] = GetDep(key)
	}
	return deps
}

// TestMockAllDeps marks all the dependencies of this package as installed
// in the right version
func TestMockAllDeps(t *testing.T) {
	t.Helper()

	// mock functions
	versionFunc := func(dep *Dependency) (*semver.Version, error) {
		return &dep.MinVersion, nil
	}
	installedFunc := func(dep *Dependency) bool {
		return true
	}

	// this functions would look for/use the actual commands,
	// so they needed to be replaced with mocks
	for _, dep := range deps {
		dep.GetVersion = versionFunc
		dep.Installed = installedFunc
	}
}

// OverwriteGetVersionWith0 marks the specified dependency as installed
// in version 0.0.0
func OverwriteGetVersionWith0(dep *Dependency) *semver.Version {
	version := semver.MustParse("0.0.0")
	dep.GetVersion = func(d *Dependency) (*semver.Version, error) {
		return version, nil
	}
	return version
}

// OverwriteUninstalled marks the specified dependency as uninstalled
func OverwriteUninstalled(dep *Dependency) {
	dep.Installed = func(d *Dependency) bool {
		return false
	}
}
