package messaging

import (
	"fmt"
	"net/url"
	"os"

	"code-intelligence.com/cifuzz/pkg/log"
)

func ShowServerConnectionMessage(server string) *url.Values {

	messagesAndParams := []struct {
		message          string
		additionalParams url.Values
	}{
		{
			message: fmt.Sprintf(`Do you want to persist your findings?
Authenticate with the CI Fuzz Server %s to get more insights.`, server),
			additionalParams: url.Values{
				"utm_source":   []string{"cli"},
				"utm_campaign": []string{"login-message"},
				"utm_term":     []string{"a"}},
		},
		{
			message: `Code Intelligence provides you with a full history for all your findings
and allows you to postpone work on findings and completely ignore
them to keep the output clean and focused.

With a free authentication you receive detailed information and solution tips
for each finding in your console.
	
All your finding data stays only with us at Code Intelligence
and will never be shared.`,
			additionalParams: url.Values{
				"utm_source":   []string{"cli"},
				"utm_campaign": []string{"login-message"},
				"utm_term":     []string{"b"},
			},
		},
	}

	messageIndex, err := pickNumberForMessagingIndex(len(messagesAndParams))
	if err != nil {
		messageIndex = 0
	}

	log.Notef(messagesAndParams[messageIndex].message)
	return &messagesAndParams[messageIndex].additionalParams
}

// To avoid that a user sees a different message each time
// we compute a stable "random" number
func pickNumberForMessagingIndex(numberOfMessages int) (int, error) {
	// Path name for the executable
	path, err := os.Executable()

	if err != nil {
		return 0, err
	}

	file, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	// We are using minute instead of hour
	// to avoid a "bias" for people trying
	// things out late in the evening
	return file.ModTime().Minute() % numberOfMessages, nil
}
