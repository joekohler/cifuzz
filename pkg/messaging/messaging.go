package messaging

import (
	"net/url"
	"os"

	"code-intelligence.com/cifuzz/pkg/log"
)

type MessagingContext string

const (
	Run     MessagingContext = "Run"
	Finding MessagingContext = "Finding"
)

var messagesAndParams = map[MessagingContext][]struct {
	message          string
	additionalParams url.Values
}{
	Run: {
		{
			message: `Do you want to persist your findings?
Authenticate with the CI App server to get more insights.`,
			additionalParams: url.Values{
				"utm_source":   []string{"cli"},
				"utm_campaign": []string{"run-message"},
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
				"utm_campaign": []string{"run-message"},
				"utm_term":     []string{"b"},
			},
		},
	},
	Finding: {
		{
			message: `Authenticate with CI App to get more insights
on your findings and persist them for a full history.`,
			additionalParams: url.Values{
				"utm_source":   []string{"cli"},
				"utm_campaign": []string{"finding-message"},
				"utm_term":     []string{"a"}},
		},
		{
			message: `With a free authentication you receive detailed information such as severity
and solution tips for each finding in your console.

All your finding data stays only with us at Code Intelligence and will never be shared.`,
			additionalParams: url.Values{
				"utm_source":   []string{"cli"},
				"utm_campaign": []string{"finding-message"},
				"utm_term":     []string{"b"},
			},
		},
	},
}

func ShowServerConnectionMessage(server string, context MessagingContext) *url.Values {
	messageAndParam, entryPresent := messagesAndParams[context]

	if !entryPresent {
		return &url.Values{}
	}

	messageIndex, err := pickNumberForMessagingIndex(len(messagesAndParams))
	if err != nil {
		messageIndex = 0
	}

	log.Notef(messageAndParam[messageIndex].message)
	return &messageAndParam[messageIndex].additionalParams
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
