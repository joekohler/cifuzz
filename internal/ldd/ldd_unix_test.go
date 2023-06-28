//go:build freebsd || linux

package ldd

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: determine if this is needed
// The reason for this test is to trigger an error in external library
// we were facing a lot recently
// It is more about reproducing the error and ensuring a helpful
// error message than actually testing our code
func TestBrokenSharedObject(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip()
	}
	// this executable is changed in a way that it is
	// missing a shared object. You can look at it with
	// `ldd testdata/my_fuzz_test`
	_, err := NonSystemSharedLibraries("testdata/my_fuzz_test")
	require.Error(t, err)
	assert.ErrorContains(t, err, "foobar.so")

}
