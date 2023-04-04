package errorid

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/pkg/finding"
)

func TestForFinding(t *testing.T) {

	testCases := []struct {
		id string
		f  *finding.Finding
	}{
		{id: "alloc_dealloc_mismatch", f: &finding.Finding{Details: "attempting free on address which was not malloc()-ed: 0x7ffebd8d4e10 in thread T0"}},
		{id: "double_free", f: &finding.Finding{Details: "attempting double-free on 0x6020000422b0 in thread T0:"}},
		{id: "deadly_signal", f: &finding.Finding{Details: "deadly signal"}},
		{id: "heap_buffer_overflow", f: &finding.Finding{Details: "heap-buffer-overflow on address 0x602000000e31 at pc 0x55657aa63e9f bp 0x7ffdae3791b0 sp 0x7ffdae378970"}},
		{id: "global_buffer_overflow", f: &finding.Finding{Details: "global-buffer-overflow on address 0x00"}},
		{id: "java_assertion_error", f: &finding.Finding{Details: "Java Assertion Error"}},
		{id: "out_of_bounds", f: &finding.Finding{Details: "java.lang.ArrayIndexOutOfBoundsException"}},
		{id: "out_of_bounds", f: &finding.Finding{Details: "undefined behavior: index 12 out of bounds for type 'int[4]'"}},
		{id: "out_of_memory", f: &finding.Finding{Details: "out-of-memory"}},
		{id: "remote_code_execution", f: &finding.Finding{Details: "Security Issue: Remote Code Execution"}},
		{id: "segmentation_fault", f: &finding.Finding{Details: "SEGV on unknown address"}},
		{id: "signed_integer_overflow", f: &finding.Finding{Details: "undefined behavior: signed integer overflow"}},
		{id: "slow_input", f: &finding.Finding{Details: "Slow input detected. Processing time: 10s"}},
		{id: "stack_buffer_overflow", f: &finding.Finding{Details: "stack-buffer-overflow on address"}},
		{id: "timeout", f: &finding.Finding{Details: "timeout after 30 seconds"}},
		{id: "use_of_uninitialized_value", f: &finding.Finding{Details: "use-of-uninitialized-value"}},

		{f: &finding.Finding{Details: "Security Issue: FooBar"}, id: "jazzer_security_issue"},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			assert.Equal(t, tc.id, ForFinding(tc.f))
		})
	}
}
