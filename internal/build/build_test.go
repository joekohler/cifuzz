package build

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/envutil"
)

func TestCommonBuildEnv_SetClang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("We are using clang-cl for windows")
	}

	os.Setenv("CC", "")
	os.Setenv("CXX", "")

	env, err := CommonBuildEnv()
	require.NoError(t, err)
	assert.Equal(t, "clang", envutil.Getenv(env, "CC"))
	assert.Equal(t, "clang++", envutil.Getenv(env, "CXX"))
}

func TestCommonBuildEnv_ClangDontOverwrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("We are using clang-cl for windows")
	}

	os.Setenv("CC", "/my/clang")
	os.Setenv("CXX", "/my/clang++")

	env, err := CommonBuildEnv()
	require.NoError(t, err)
	assert.Equal(t, "/my/clang", envutil.Getenv(env, "CC"))
	assert.Equal(t, "/my/clang++", envutil.Getenv(env, "CXX"))
}
