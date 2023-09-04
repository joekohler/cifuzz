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
)

// NoValidTokenError indicates that no valid API access token is configured.
type NoValidTokenError struct {
	err error
}

func (e NoValidTokenError) Error() string {
	return e.err.Error()
}

func (e NoValidTokenError) Unwrap() error {
	return e.err
}

// GetToken returns the API access token for the given server.
func GetToken(server string) (string, error) {
	// Try the environment variable
	token := os.Getenv("CIFUZZ_API_TOKEN")
	if token != "" {
		log.Print("Using token from $CIFUZZ_API_TOKEN")
		return token, nil
	}

	// Try the access tokens config file
	return tokenstorage.Get(server)
}

// HasValidToken returns true if a valid API access token is configured
// for the given server.
func HasValidToken(server string) (bool, error) {
	_, err := GetValidToken(server)
	var noValidTokenError *NoValidTokenError
	if errors.As(err, &noValidTokenError) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetValidToken returns the API access token for the given server if it
// is valid. If no valid token is found, NoValidTokenError is returned.
func GetValidToken(server string) (string, error) {
	token, err := GetToken(server)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", &NoValidTokenError{errors.New("Please log in with a valid API access token")}
	}

	apiClient := api.APIClient{Server: server}
	err = EnsureValidToken(apiClient, token)
	if err != nil {
		return "", err
	}

	return token, nil
}

// readTokenInteractively reads the API access token from the user with an
// interactive dialog prompt.
func readTokenInteractively(server string) (string, error) {
	path, err := url.JoinPath(server, "dashboard", "settings", "account", "tokens")
	if err != nil {
		return "", errors.WithStack(err)
	}

	values := url.Values{}
	values.Set("create", "")
	values.Add("origin", "cli")

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
func ReadCheckAndStoreTokenInteractively(apiClient *api.APIClient) (string, error) {
	token, err := readTokenInteractively(apiClient.Server)
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
	if isValid {
		log.Success("You are authenticated.")
		return nil
	}

	log.Warn(`Failed to authenticate with the configured API access token.
It's possible that the token has been revoked.`)

	if !viper.GetBool("interactive") || !term.IsTerminal(int(os.Stdin.Fd())) {
		return &NoValidTokenError{errors.New("Please log in with a valid API access token")}
	}

	tryAgain, err := dialog.Confirm("Do you want to log in again?", true)
	if err != nil {
		return err
	}
	if !tryAgain {
		return &NoValidTokenError{errors.New("Please log in with a valid API access token")}
	}

	_, err = ReadCheckAndStoreTokenInteractively(&apiClient)
	return err
}

// ShowServerConnectionDialog ask users if they want to use a SaaS backend
// if they are not authenticated and returns their wish to authenticate.
func ShowServerConnectionDialog(server string) (bool, error) {
	wishToAuthenticate, err := dialog.Confirm("Do you want to authenticate?", true)
	if err != nil {
		return false, err
	}

	if wishToAuthenticate {
		apiClient := api.APIClient{Server: server}
		_, err := ReadCheckAndStoreTokenInteractively(&apiClient)
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
	err = tokenstorage.Set(apiClient.Server, token)
	if err != nil {
		return err
	}
	tokenFilePath, err := tokenstorage.GetTokenFilePath()
	if err != nil {
		return err
	}
	log.Successf("Successfully authenticated with %s", apiClient.Server)
	log.Infof("Your API access token is stored in %s", tokenFilePath)
	return nil
}
