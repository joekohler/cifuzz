package errorid

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"code-intelligence.com/cifuzz/pkg/report"
)

func TestForFinding(t *testing.T) {

	testCases := []struct {
		id string
		f  *report.Finding
	}{
		{id: "alloc_dealloc_mismatch", f: &report.Finding{Details: "attempting free on address which was not malloc()-ed: 0x7ffebd8d4e10 in thread T0"}},
		{id: "double_free", f: &report.Finding{Details: "attempting double-free on 0x6020000422b0 in thread T0:"}},
		{id: "deadly_signal", f: &report.Finding{Details: "deadly signal"}},
		{id: "heap_buffer_overflow", f: &report.Finding{Details: "heap-buffer-overflow on address 0x602000000e31 at pc 0x55657aa63e9f bp 0x7ffdae3791b0 sp 0x7ffdae378970"}},
		{id: "global_buffer_overflow", f: &report.Finding{Details: "global-buffer-overflow on address 0x00"}},
		{id: "java_assertion_error", f: &report.Finding{Details: "Java Assertion Error"}},
		{id: "java_out_of_bounds", f: &report.Finding{Details: "java.lang.ArrayIndexOutOfBoundsException"}},
		{id: "out_of_bounds", f: &report.Finding{Details: "undefined behavior: index 12 out of bounds for type 'int[4]'"}},
		{id: "out_of_memory", f: &report.Finding{Details: "out-of-memory"}},
		{id: "remote_code_execution", f: &report.Finding{Details: "Security Issue: Remote Code Execution"}},
		{id: "segmentation_fault", f: &report.Finding{Details: "SEGV on unknown address"}},
		{id: "shift_exponent", f: &report.Finding{Details: "undefined behavior: shift exponent 32 is too large for 32-bit type 'int'"}},
		{id: "signed_integer_overflow", f: &report.Finding{Details: "undefined behavior: signed integer overflow"}},
		{id: "slow_input", f: &report.Finding{Details: "Slow input detected. Processing time: 10s"}},
		{id: "stack_buffer_overflow", f: &report.Finding{Details: "stack-buffer-overflow on address"}},
		{id: "timeout", f: &report.Finding{Details: "timeout after 30 seconds"}},
		{id: "use_of_uninitialized_value", f: &report.Finding{Details: "use-of-uninitialized-value"}},
		{id: "java_exception", f: &report.Finding{Details: "java.lang.Exception"}},
		{id: "java_exception", f: &report.Finding{Details: "java.lang.SecurityException"}},

		{f: &report.Finding{Details: "Security Issue: FooBar"}, id: "jazzer_security_issue"},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			assert.Equal(t, tc.id, ForFinding(tc.f))
		})
	}
}
