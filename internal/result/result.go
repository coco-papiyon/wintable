// Package result owns credential-free task results, aggregation and rendering.
// It does not depend on transport connection settings.
package result

type Status string

const (
	OK          Status = "ok"
	Changed     Status = "changed"
	Failed      Status = "failed"
	Unreachable Status = "unreachable"
	Skipped     Status = "skipped"
	Cancelled   Status = "cancelled"
)

// Error contains a classified, sanitized code, never raw lower-level errors.
type Error struct {
	Code string `json:"code"`
}

// TaskResult preserves unknown changes and unavailable exit codes as JSON null.
// Unexecuted tasks have Changed=false and ExitCode=nil; interrupted execution
// has Changed=nil. TaskIndex is zero-based and DurationMS is milliseconds.
// Optional output bodies are base64 strings, populated only with explicit opt-in.
type TaskResult struct {
	Host            string  `json:"host"`
	Task            string  `json:"task"`
	TaskIndex       int     `json:"task_index"`
	Status          Status  `json:"status"`
	Changed         *bool   `json:"changed"`
	ExitCode        *int    `json:"exit_code"`
	DurationMS      int64   `json:"duration_ms"`
	Error           *Error  `json:"error,omitempty"`
	StdoutBase64    *string `json:"stdout_base64,omitempty"`
	StderrBase64    *string `json:"stderr_base64,omitempty"`
	StdoutTruncated bool    `json:"stdout_truncated"`
	StderrTruncated bool    `json:"stderr_truncated"`
}
