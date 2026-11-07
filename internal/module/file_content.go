package module

import (
	"bytes"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

// DecodeFileContent accepts only the fixed receiver's complete response forms.
// File modules have no child exit code or captured content.
func DecodeFileContent(r transport.Response) result.TaskResult {
	fail := func(known bool) result.TaskResult {
		row := result.TaskResult{Status: result.Failed, Error: result.PublicError("task_failed")}
		if known {
			b := false
			row.Changed = &b
		}
		return row
	}
	if r.Communication != transport.CommunicationComplete || r.ExitCode == nil || *r.ExitCode != 0 || r.StdoutTruncated || r.StderrTruncated || len(r.Stderr) != 0 {
		return fail(false)
	}
	switch string(bytes.TrimSpace(r.Stdout)) {
	case `{"changed":false}`:
		b := false
		return result.TaskResult{Status: result.OK, Changed: &b}
	case `{"changed":true}`:
		b := true
		return result.TaskResult{Status: result.Changed, Changed: &b}
	case `{"error":"invalid_request"}`, `{"error":"write_failed"}`:
		return fail(true)
	default:
		return fail(false)
	}
}
