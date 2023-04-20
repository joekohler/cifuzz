package tokenstorage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/util/fileutil"
)

var accessTokens map[string]string

var (
	configDir, err       = os.UserConfigDir()
	accessTokensFilePath = filepath.Join(configDir, "cifuzz", "access_tokens.json")
)

func init() {
	migrateOldTokens()

	// Expand the $HOME environment variable in the access tokens file path
	accessTokensFilePath = os.ExpandEnv(accessTokensFilePath)
	if err != nil {
		log.Errorf(err, "Error getting user config directory: %v", err.Error())
	}

	var err error
	bytes, err := os.ReadFile(accessTokensFilePath)
	if err != nil && os.IsNotExist(err) {
		// The access tokens file doesn't exist, so we initialize the
		// access tokens with an empty map
		accessTokens = map[string]string{}
		return
	}
	if err != nil {
		log.Errorf(err, "Error reading access tokens file: %v", err.Error())
	}
	err = json.Unmarshal(bytes, &accessTokens)
	if err != nil {
		log.Errorf(err, "Error parsing access tokens: %v", err.Error())
	}
}

func Set(target, token string) error {
	// Ensure that the parent directory exists
	err := os.MkdirAll(filepath.Dir(accessTokensFilePath), 0o755)
	if err != nil {
		return errors.WithStack(err)
	}

	accessTokens[target] = token

	// Convert the access tokens to JSON
	bytes, err := json.MarshalIndent(accessTokens, "", "  ")
	if err != nil {
		return errors.WithStack(err)
	}

	// Write the JSON to file
	err = os.WriteFile(accessTokensFilePath, bytes, 0o600)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get returns the access token for the given target
// If the given target doesn't exist, try to add or remove a trailing slash
// and return the access token for that target
func Get(target string) string {
	if token, ok := accessTokens[target]; ok {
		return token
	}
	if token, ok := accessTokens[strings.TrimSuffix(target, "/")]; ok {
		return token
	}
	if token, ok := accessTokens[target+"/"]; ok {
		return token
	}
	return ""
}

func GetServerURLs() []string {
	var serverURLs []string
	for target := range accessTokens {
		serverURLs = append(serverURLs, target)
	}
	return serverURLs
}

func GetTokenFilePath() string {
	return accessTokensFilePath
}

// migrateOldTokens migrates the old access tokens file to the new location
func migrateOldTokens() {
	oldTokensFilePath := os.ExpandEnv("$HOME/.config/cifuzz/access_tokens.json")

	// make sure that new tokens file directory exists
	err := os.MkdirAll(filepath.Dir(accessTokensFilePath), 0o755)
	if err != nil {
		log.Errorf(err, "Error creating config directory: %v", err.Error())
	}

	exists, err := fileutil.Exists(oldTokensFilePath)
	if err != nil {
		log.Errorf(err, "Error checking if old tokens file exists: %v", err.Error())
	}

	if exists && oldTokensFilePath != accessTokensFilePath {
		log.Infof("Migrating old tokens file to new location: %s", accessTokensFilePath)
		err := os.Rename(oldTokensFilePath, accessTokensFilePath)
		if err != nil {
			log.Errorf(err, "Error migrating old tokens file: %v", err.Error())
		}
	}
}
