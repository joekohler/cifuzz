package auth

import (
	"net/url"
	"os"

	"github.com/pkg/browser"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"code-intelligence.com/cifuzz/internal/api"
	"code-intelligence.com/cifuzz/internal/tokenstorage"
	"code-intelligence.com/cifuzz/pkg/dialog"
	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/messaging"
)

// GetToken returns the API access token for the given server.
func GetToken(server string) string {
	// Try the environment variable
	token := os.Getenv("CIFUZZ_API_TOKEN")
	if token != "" {
		log.Print("Using token from $CIFUZZ_API_TOKEN")
		return token
	}

	// Try the access tokens config file
	return tokenstorage.Get(server)
}

// GetAuthStatus returns the authentication status of the user
// for the given server.
func GetAuthStatus(server string) (bool, error) {
	// Obtain the API access token
	token := GetToken(server)

	if token == "" {
		return false, nil
	}

	// Token might be invalid, so try to authenticate with it
	apiClient := api.APIClient{Server: server}
	err := EnsureValidToken(apiClient, token)
	if err != nil {
		return false, err
	}

	return true, nil
}

// readTokenInteractively reads the API access token from the user with an
// interactive dialog prompt.
func readTokenInteractively(server string, additionalParams *url.Values) (string, error) {
	path, err := url.JoinPath(server, "dashboard", "settings", "account", "tokens")
	if err != nil {
		return "", errors.WithStack(err)
	}

	values := url.Values{}
	values.Set("create", "")
	values.Add("origin", "cli")

	// Add additional params to existing values
	if additionalParams != nil {
		for key, params := range *additionalParams {
			for _, param := range params {
				values.Add(key, param)
			}
		}
	}

	url, err := url.Parse(path)
	if err != nil {
		return "", errors.WithStack(err)
	}
	url.RawQuery = values.Encode()

	log.Printf("You need an API access token which can be generated here:\n%s", url.String())

	openBrowser, err := dialog.Confirm("Open browser to generate a new token?", true)
	if err != nil {
		return "", err
	}

	if openBrowser {
		err = browser.OpenURL(url.String())
		if err != nil {
			err = errors.WithStack(err)
			log.Errorf(err, "Failed to open browser: %v", err)
		}
	}

	token, err := dialog.ReadSecret("Paste your access token")
	if err != nil {
		return "", err
	}

	return token, nil
}

// ReadCheckAndStoreTokenInteractively reads the API access token from the
// user, checks if it is valid and stores it.
func ReadCheckAndStoreTokenInteractively(apiClient *api.APIClient, additionalParams *url.Values) (string, error) {
	token, err := readTokenInteractively(apiClient.Server, additionalParams)
	if err != nil {
		return "", err
	}

	err = CheckAndStoreToken(apiClient, token)
	if err != nil {
		return "", err
	}

	return token, nil
}

// EnsureValidToken checks if the token is valid and asks the user to log in
// again if it is not.
func EnsureValidToken(apiClient api.APIClient, token string) error {
	isValid, err := apiClient.IsTokenValid(token)
	if err != nil {
		return err
	}
	if !isValid {
		log.Warn(`Failed to authenticate with the configured API access token.
It's possible that the token has been revoked.`)

		if viper.GetBool("interactive") && term.IsTerminal(int(os.Stdin.Fd())) {
			tryAgain, err := dialog.Confirm("Do you want to log in again?", true)
			if err != nil {
				return err
			}
			if tryAgain {
				_, err = ReadCheckAndStoreTokenInteractively(&apiClient, nil)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ShowServerConnectionDialog ask users if they want to use a SaaS backend
// if they are not authenticated and returns their wish to authenticate.
func ShowServerConnectionDialog(server string, context messaging.MessagingContext) (bool, error) {
	additionalParams := messaging.ShowServerConnectionMessage(server, context)

	wishToAuthenticate, err := dialog.Confirm("Do you want to authenticate?", true)
	if err != nil {
		return false, err
	}

	if wishToAuthenticate {
		apiClient := api.APIClient{Server: server}
		_, err := ReadCheckAndStoreTokenInteractively(&apiClient, additionalParams)
		if err != nil {
			return false, err
		}
	}

	return wishToAuthenticate, nil
}

// CheckAndStoreToken checks if the token is valid and stores it if it is.
func CheckAndStoreToken(apiClient *api.APIClient, token string) error {
	err := EnsureValidToken(*apiClient, token)
	if err != nil {
		return err
	}
	log.Successf("Successfully authenticated with %s", apiClient.Server)
	log.Infof("Your API access token is stored in %s", tokenstorage.GetTokenFilePath())
	return nil
}
