package api

import (
	"encoding/json"
	"io"
	"net/url"

	"github.com/pkg/errors"

	"code-intelligence.com/cifuzz/pkg/finding"
	"code-intelligence.com/cifuzz/pkg/log"
)

type errorDetailsJSON struct {
	VersionSchema int                    `json:"version_schema"`
	ErrorDetails  []finding.ErrorDetails `json:"error_details"`
}

// GetErrorDetails gets the error details from the API
func (client *APIClient) GetErrorDetails(token string) ([]finding.ErrorDetails, error) {
	// get it from the API
	url, err := url.JoinPath("v2", "error-details")
	if err != nil {
		return nil, err
	}

	resp, err := client.sendRequest("GET", url, nil, token)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// the request did not succeed, but we don't want the entire process to fail
		// so we just log the error and return an empty list
		if resp.StatusCode == 401 {
			log.Warnf("Not authorized to get error details. Please log in to enable this feature.")
		} else {
			log.Warnf("Failed to get error details: %s", resp.Status)
			log.Infof("Response: %s", resp.Body)
		}
		log.Infof("Continuing without external error details")
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var errorDetailsFromJSON errorDetailsJSON
	err = json.Unmarshal(body, &errorDetailsFromJSON)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return errorDetailsFromJSON.ErrorDetails, nil
}
