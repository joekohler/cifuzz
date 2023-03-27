package auth

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/spf13/viper"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/cmdutils"
	"code-intelligence.com/cifuzz/internal/cmdutils/login"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
	"code-intelligence.com/cifuzz/pkg/cicheck"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
)

// IsAuthenticated checks if the user is authenticated with the server.
func IsAuthenticated(server string) (bool, error) {
	interactive := viper.GetBool("interactive")
	if cicheck.IsCIEnvironment() {
		interactive = false
	}

	// Check if the server option is a valid URL
	err := api.ValidateURL(server)
	if err != nil {
		// See if prefixing https:// makes it a valid URL
		err = api.ValidateURL("https://" + server)
		if err != nil {
			log.Error(err, fmt.Sprintf("server %q is not a valid URL", server))
		}
		server = "https://" + server
	}

	// normalize server URL
	url, err := url.JoinPath(server)
	if err != nil {
		return false, cmdutils.WrapSilentError(err)
	}
	server = url

	authenticated, err := GetAuthStatus(server)
	if err != nil {
		var connErr *api.ConnectionError
		if errors.As(err, &connErr) {
			log.Warn("Connection to API failed. Skipping sync.")
			log.Debugf("Connection error: %s (continuing gracefully)", connErr)
			return false, nil
		} else {
			fmt.Println("AUTH STATUS CHECK ERROR")
			return false, cmdutils.WrapSilentError(err)
		}
	}

	if interactive && !authenticated {
		// establish server connection to check user auth
		authenticated, err = ShowServerConnectionDialog(server)
		if err != nil {
			var connErr *api.ConnectionError
			if errors.As(err, &connErr) {
				log.Warn("Connection to API failed. Skipping sync.")
				log.Debugf("Connection error: %v (continuing gracefully)", connErr)
				return false, nil
			} else {
				return false, cmdutils.WrapSilentError(err)
			}
		}
	}
	return authenticated, nil
}

func GetAuthStatus(server string) (bool, error) {
	// Obtain the API access token
	token := login.GetToken(server)

	if token == "" {
		return false, nil
	}

	// Token might be invalid, so try to authenticate with it
	apiClient := api.APIClient{Server: server}
	err := login.CheckValidToken(&apiClient, token)
	if err != nil {
		log.Warnf(`Failed to authenticate with the configured API access token.
It's possible that the token has been revoked. Please try again after
removing the token from %s.`, tokenstorage.GetTokenFilePath())

		return false, err
	}

	return true, nil
}

// ShowServerConnectionDialog ask users if they want to use a SaaS backend
// if they are not authenticated and returns their wish to authenticate
func ShowServerConnectionDialog(server string) (bool, error) {
	additionalParams := messaging.ShowServerConnectionMessage(server)

	wishToAuthenticate, err := dialog.Confirm("Do you want to authenticate?", true)
	if err != nil {
		return false, err
	}

	if wishToAuthenticate {
		apiClient := api.APIClient{Server: server}
		_, err := login.ReadCheckAndStoreTokenInteractively(&apiClient, additionalParams)
		if err != nil {
			return false, err
		}
	}

	return wishToAuthenticate, nil
}
