package e2e

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/cmdutils/auth"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
	"code-intelligence.com/cifuzz/pkg/cicheck"
)

// Convenience option for local testing. Grab local token, backup the existing one and restore it after the test.
func TestUseLocalAPIToken(t *testing.T) {
	t.Helper()
	if !cicheck.IsCIEnvironment() && os.Getenv(envvarWithE2EUserToken) == "" {
		fmt.Println("E2E_TEST_CIFUZZ_API_TOKEN envvar is not set. Trying to use the default one, since this is not a CI/CD run.")
		token, err := auth.GetToken(ciServerToUseForE2ETests)
		require.NoError(t, err)
		if token != "" {
			fmt.Println("Found local token with login.GetToken, going to use it for the tests.")
			t.Setenv(envvarWithE2EUserToken, token)
			tokenFilePath, err := tokenstorage.GetTokenFilePath()
			require.NoError(t, err)
			if _, err := os.Stat(tokenFilePath); err == nil {
				fmt.Println("Backing up existing access token file")
				err = os.Rename(tokenFilePath, tokenFilePath+".bak")
				require.NoError(t, err)
				defer func() {
					err = os.Rename(tokenFilePath+".bak", tokenFilePath)
					require.NoError(t, err)
				}()
			}
		}
	}
}
