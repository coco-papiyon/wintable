package module

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

// DecodeCommand treats the outer PowerShell exit code separately from the child.
// Malformed, incomplete and contradictory responses never reveal response text.
func DecodeCommand(r transport.Response, output bool) result.TaskResult {
	fail := func(code string, changed *bool) result.TaskResult {
		return result.TaskResult{Status: result.Failed, Changed: changed, Error: result.PublicError(code)}
	}
	if r.Communication != transport.CommunicationComplete || r.ExitCode == nil || *r.ExitCode != 0 || r.StdoutTruncated || len(r.Stderr) != 0 || r.StderrTruncated || !utf8.Valid(r.Stdout) {
		return fail("task_failed", nil)
	}
	d := json.NewDecoder(bytes.NewReader(r.Stdout))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return fail("task_failed", nil)
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return fail("task_failed", nil)
		}
		key, ok := k.(string)
		if !ok {
			return fail("task_failed", nil)
		}
		if _, ok := fields[key]; ok {
			return fail("task_failed", nil)
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return fail("task_failed", nil)
		}
		fields[key] = raw
	}
	if _, err = d.Token(); err != nil {
		return fail("task_failed", nil)
	}
	if _, err = d.Token(); err != io.EOF {
		return fail("task_failed", nil)
	}
	if raw, ok := fields["error"]; ok {
		var code string
		if len(fields) != 1 || json.Unmarshal(raw, &code) != nil {
			return fail("task_failed", nil)
		}
		switch code {
		case "invalid_request", "start_failed":
			b := false
			return fail("task_failed", &b)
		case "execution_failed":
			return fail("task_failed", nil)
		default:
			return fail("task_failed", nil)
		}
	}
	if len(fields) != 5 {
		return fail("task_failed", nil)
	}
	var exit int32
	var stdout, stderr string
	var oc, ec bool
	targets := map[string]any{"exit_code": &exit, "stdout": &stdout, "stderr": &stderr, "stdout_truncated": &oc, "stderr_truncated": &ec}
	for k, v := range targets {
		raw, ok := fields[k]
		if !ok || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, v) != nil {
			return fail("task_failed", nil)
		}
	}
	a, e := base64.StdEncoding.Strict().DecodeString(stdout)
	if e != nil || len(a) > transport.CaptureLimit || base64.StdEncoding.EncodeToString(a) != stdout {
		return fail("task_failed", nil)
	}
	b, e := base64.StdEncoding.Strict().DecodeString(stderr)
	if e != nil || len(b) > transport.CaptureLimit || base64.StdEncoding.EncodeToString(b) != stderr || (oc && len(a) != transport.CaptureLimit) || (ec && len(b) != transport.CaptureLimit) {
		return fail("task_failed", nil)
	}
	code := int(exit)
	row := result.TaskResult{Status: result.Changed, ExitCode: &code}
	if code == 0 {
		changed := true
		row.Changed = &changed
	} else {
		row.Status = result.Failed
		row.Error = result.PublicError("task_failed")
	}
	if output {
		row.StdoutBase64 = &stdout
		row.StderrBase64 = &stderr
		row.StdoutTruncated = oc
		row.StderrTruncated = ec
	}
	return row
}
