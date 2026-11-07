package module

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

func TestCommandResponse(t *testing.T) {
	zero := 0
	wrap := func(body string) transport.Response {
		return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &zero, Stdout: []byte(body)}
	}
	good := `{"exit_code":0,"stdout":"AP8=","stderr":"","stdout_truncated":false,"stderr_truncated":false}`
	row := DecodeCommand(wrap(good), true)
	if row.Status != result.Changed || row.Changed == nil || !*row.Changed || row.ExitCode == nil || *row.ExitCode != 0 || *row.StdoutBase64 != "AP8=" {
		t.Fatal(row)
	}
	row = DecodeCommand(wrap(good), false)
	if row.StdoutBase64 != nil || row.StderrBase64 != nil {
		t.Fatal("output disclosure")
	}
	for _, body := range []string{``, `[]`, `null`, good + `{}`, `{"error":null}`, `{"error":"SECRET"}`, `{"error":"invalid_request","extra":0}`, `{"exit_code":0,"exit_code":1}`, `{"exit_code":null,"stdout":"","stderr":"","stdout_truncated":false,"stderr_truncated":false}`, `{"exit_code":0,"stdout":"!!!","stderr":"","stdout_truncated":false,"stderr_truncated":false}`, `{"exit_code":0,"stdout":"","stderr":"","stdout_truncated":true,"stderr_truncated":false}`, `{"exit_code":0,"stdout":"","stderr":"","stdout_truncated":false,"extra":false}`} {
		row := DecodeCommand(wrap(body), true)
		if row.Status != result.Failed || row.Changed != nil || row.ExitCode != nil || row.StdoutBase64 != nil || row.Error.Code != "task_failed" {
			t.Fatal(row)
		}
	}
	for _, kind := range []string{"outer_exit", "truncated", "stderr", "communication", "utf8"} {
		r := wrap(good)
		switch kind {
		case "outer_exit":
			code := 1
			r.ExitCode = &code
		case "truncated":
			r.StdoutTruncated = true
		case "stderr":
			r.Stderr = []byte("SECRET")
		case "communication":
			r.Communication = transport.CommunicationInterrupted
		case "utf8":
			r.Stdout = []byte{255}
		}
		if row := DecodeCommand(r, true); row.Changed != nil || row.Status != result.Failed || row.ExitCode != nil {
			t.Fatal(kind, row)
		}
	}
	for _, code := range []string{"invalid_request", "start_failed", "execution_failed"} {
		row := DecodeCommand(wrap(`{"error":"`+code+`"}`), false)
		if (row.Changed == nil) != (code == "execution_failed") || (row.Changed != nil && *row.Changed) {
			t.Fatal(code, row)
		}
	}
	body, _ := json.Marshal(map[string]any{"exit_code": 7, "stdout": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{255}, transport.CaptureLimit)), "stderr": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0}, transport.CaptureLimit)), "stdout_truncated": true, "stderr_truncated": true})
	if len(body) <= 2*transport.CaptureLimit || len(body) > transport.WrapperResponseLimit {
		t.Fatal("envelope size")
	}
	row = DecodeCommand(wrap(string(body)), true)
	if row.ExitCode == nil || *row.ExitCode != 7 || row.Changed != nil || !row.StdoutTruncated || !row.StderrTruncated {
		t.Fatal(row)
	}
}
