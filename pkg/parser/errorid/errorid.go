package errorid

import (
	"regexp"
	"strings"

	"code-intelligence.com/cifuzz/pkg/log"
	"code-intelligence.com/cifuzz/pkg/report"
)

type matcher struct {
	id         string
	substrings []string
	regexs     []*regexp.Regexp
}

func (m *matcher) Match(input string) bool {
	for _, substring := range m.substrings {
		if strings.Contains(input, substring) {
			return true
		}
	}

	for _, regex := range m.regexs {
		if regex.MatchString(input) {
			return true
		}
	}
	return false
}

var matchers = []matcher{
	{id: "alloc_dealloc_mismatch", substrings: []string{"attempting free on address which was not malloc"}},
	{id: "deadly_signal", substrings: []string{"deadly signal"}},
	{id: "double_free", substrings: []string{"attempting double-free on"}},
	{id: "heap_buffer_overflow", substrings: []string{"heap-buffer-overflow on address"}},
	{id: "heap_use_after_free", substrings: []string{"heap-use-after-free on address"}},
	{id: "global_buffer_overflow", substrings: []string{"global-buffer-overflow on address"}},
	{id: "java_assertion_error", substrings: []string{"Java Assertion Error"}},
	{
		id:         "out_of_bounds",
		substrings: []string{"java.lang.ArrayIndexOutOfBoundsException"},
		regexs:     []*regexp.Regexp{regexp.MustCompile(`undefined behavior: index \d+ out of bounds`)},
	},
	{id: "memory_leak", substrings: []string{"detected memory leaks"}},
	{id: "out_of_memory", substrings: []string{"out-of-memory"}},
	{id: "remote_code_execution", substrings: []string{"Security Issue: Remote Code Execution"}},
	{id: "segmentation_fault", substrings: []string{"SEGV on unknown address"}},
	{id: "signed_integer_overflow", substrings: []string{"undefined behavior: signed integer overflow"}},
	{id: "slow_input", substrings: []string{"Slow input detected. Processing time:"}},
	{id: "stack_buffer_overflow", substrings: []string{"stack-buffer-overflow on address"}},
	{id: "stack_exhaustion", substrings: []string{"stack-overflow on address"}},
	{id: "timeout", regexs: []*regexp.Regexp{regexp.MustCompile(`timeout after \d+ \w+`)}},
	{id: "use_after_return", substrings: []string{"stack-use-after-return on address"}},
	{id: "use_after_scope", substrings: []string{"stack-use-after-scope on address"}},
	{id: "use_of_uninitialized_value", substrings: []string{"use-of-uninitialized-value"}},

	// more global issues, should be at the end so they do not overwrite more explicit ones
	{id: "jazzer_security_issue", substrings: []string{"Security Issue:"}},
}

func ForFinding(f *report.Finding) string {
	for _, m := range matchers {
		if m.Match(f.Details) {
			return m.id
		}
	}
	log.Warnf("unable to find matching error id for given finding %s", f.Details)
	return ""
}
