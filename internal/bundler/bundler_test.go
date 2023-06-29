package bundler

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
)

func TestParsingAdditionalFilesArguments(t *testing.T) {
	viper.Set("verbose", true)

	type test struct {
		input      string
		wantSource string
		wantTarget string
	}
	tests := map[string]test{
		"one_file": {
			input:      "file.so",
			wantSource: "file.so",
			wantTarget: filepath.Join("work_dir", "file.so")},
		"one_file_in_dir": {
			input:      filepath.Join("source", "path", "file.so"),
			wantSource: filepath.Join("source", "path", "file.so"),
			wantTarget: filepath.Join("work_dir", "file.so")},
		"rename": {
			input:      filepath.Join("path", "file.so") + ";" + filepath.Join("path", "new.so"),
			wantSource: filepath.Join("path", "file.so"),
			wantTarget: filepath.Join("path", "new.so"),
		},
		"dir": {
			input:      "path",
			wantSource: "path",
			wantTarget: filepath.Join("work_dir", "path"),
		},
		"sub dir": {
			input:      filepath.Join("path", "source"),
			wantSource: filepath.Join("path", "source"),
			wantTarget: filepath.Join("work_dir", "source"),
		},
		"rename_dir": {
			input:      "source;target",
			wantSource: "source",
			wantTarget: "target",
		},
	}

	// add special windows test cases for absolute windows paths
	if runtime.GOOS == "windows" {
		tests["win_absolute_1"] = test{
			input:      filepath.Join("C:", "foo", "file.so"),
			wantSource: filepath.Join("C:", "foo", "file.so"),
			wantTarget: filepath.Join("work_dir", "file.so"),
		}
		tests["win_absolute_2"] = test{
			input:      filepath.Join("C:", "foo", "file.so") + ";new.so",
			wantSource: filepath.Join("C:", "foo", "file.so"),
			wantTarget: "new.so",
		}
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			source, target, err := parseAdditionalFilesArgument(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantSource, source)
			assert.Equal(t, tc.wantTarget, target)
		})
	}
}

// If an error occurs during bundling there should be no
// broken bundle file left
func TestRemoveBundleOnError(t *testing.T) {
	testDir := testutil.MkdirTemp(t, "", "bundle-delete-*")
	bundlePath := filepath.Join(testDir, "artifact.tar.gz")
	opts := &Opts{
		// using invalid build system to make the bundling fail
		BuildSystem: "FOO",
		OutputPath:  bundlePath,
	}
	bundler := New(opts)

	path, err := bundler.Bundle()
	require.Empty(t, path)
	require.Error(t, err)

	assert.NoFileExists(t, bundlePath)
}
