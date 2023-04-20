package tokenstorage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/util/fileutil"
)

func TestGetAndSet(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "access-tokens-test-")
	require.NoError(t, err)
	defer fileutil.Cleanup(tempDir)
	accessTokensFilePath = filepath.Join(tempDir, "access_tokens.json")
	accessTokens = map[string]string{}

	token := Get("http://localhost:8000")
	require.Empty(t, token)

	err = Set("http://localhost:8000", "token")
	require.NoError(t, err)

	token = Get("http://localhost:8000")
	require.Equal(t, "token", token)

	err = Set("http://localhost:8000", "token2")
	require.NoError(t, err)

	token = Get("http://localhost:8000")
	require.NoError(t, err)
	require.Equal(t, "token2", token)
}

func TestGet(t *testing.T) {
	accessTokens = map[string]string{
		"app.example.com":                    "123",
		"app.code-intelligence.com":          "456",
		"app.staging.code-intelligence.com/": "789",
	}

	// Test exact match
	token := Get("app.example.com")
	require.Equal(t, "123", token)

	// Test target with trailing slash (should match the same as without)
	token = Get("app.code-intelligence.com/")
	require.Equal(t, "456", token)

	// Test target without trailing slash (should match the same as with)
	token = Get("app.staging.code-intelligence.com")
	require.Equal(t, "789", token)

	// Test non-existing target
	token = Get("example.org")
	require.Empty(t, token)
}
