package module

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

func TestLinuxDecoder(t *testing.T) {
	zero := 0
	envelope := func(s string) transport.Response {
		return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &zero, Stdout: []byte(s)}
	}
	valid := `{"exit_code":0,"stdout":"","stderr":"","stdout_truncated":false,"stderr_truncated":false}`
	for _, code := range []int{-64, -15, -1, 0, 1, 255} {
		row := DecodeLinuxCommand(envelope(strings.Replace(valid, `"exit_code":0`, fmt.Sprintf(`"exit_code":%d`, code), 1)), true)
		if row.ExitCode == nil || *row.ExitCode != code || (code == 0 && (row.Status != result.Changed || row.Changed == nil || !*row.Changed)) || (code != 0 && (row.Status != result.Failed || row.Changed != nil)) {
			t.Fatal(code, row)
		}
	}
	for _, code := range []string{"invalid_request", "start_failed", "execution_failed", "unknown"} {
		row := DecodeLinuxCommand(envelope(fmt.Sprintf(`{"error":%q}`, code)), false)
		known := code == "invalid_request" || code == "start_failed"
		if row.Status != result.Failed || row.ExitCode != nil || (known && (row.Changed == nil || *row.Changed)) || (!known && row.Changed != nil) {
			t.Fatal(code, row)
		}
	}
	bad := []string{"", valid + valid, strings.Replace(valid, `"exit_code":0`, `"exit_code":0,"exit_code":0`, 1), strings.Replace(valid, `"exit_code":0`, `"exit_code":256`, 1), strings.Replace(valid, `"exit_code":0`, `"exit_code":-65`, 1), strings.Replace(valid, `"exit_code":0`, `"exit_code":null`, 1), strings.Replace(valid, `"stdout":""`, `"stdout":"YQ==\n"`, 1), strings.Replace(valid, `"stdout":""`, `"stdout":null`, 1), strings.Replace(valid, `"stdout_truncated":false`, `"stdout_truncated":true`, 1), strings.Replace(valid, `"stderr_truncated":false`, `"other":false`, 1), `{"error":"start_failed","x":0}`, strings.Repeat(" ", transport.WrapperResponseLimit) + valid}
	for i, s := range bad {
		row := DecodeLinuxCommand(envelope(s), true)
		if row.Status != result.Failed || row.Changed != nil || row.ExitCode != nil || row.StdoutBase64 != nil {
			t.Fatal(i, row)
		}
	}
	for _, mutate := range []func(*transport.Response){func(r *transport.Response) { x := 1; r.ExitCode = &x }, func(r *transport.Response) { r.Stderr = []byte("secret") }, func(r *transport.Response) { r.StdoutTruncated = true }, func(r *transport.Response) { r.Communication = "" }} {
		r := envelope(valid)
		mutate(&r)
		if row := DecodeLinuxCommand(r, true); row.Changed != nil || row.ExitCode != nil {
			t.Fatal(row)
		}
	}
	for _, n := range []int{transport.CaptureLimit - 1, transport.CaptureLimit, transport.CaptureLimit + 1} {
		s := strings.Replace(valid, `"stdout":""`, `"stdout":"`+base64.StdEncoding.EncodeToString(make([]byte, n))+`"`, 1)
		row := DecodeLinuxCommand(envelope(s), true)
		if (row.ExitCode != nil) != (n <= transport.CaptureLimit) {
			t.Fatal(n, row.Status)
		}
	}
	if r := DecodeLinuxCommand(envelope(valid), false); r.StdoutBase64 != nil || r.StderrBase64 != nil {
		t.Fatal("output exposed")
	}
	for _, n := range []int{transport.WrapperResponseLimit - 1, transport.WrapperResponseLimit, transport.WrapperResponseLimit + 1} {
		r := DecodeLinuxCommand(envelope(valid+strings.Repeat(" ", n-len(valid))), false)
		if (r.ExitCode != nil) != (n <= transport.WrapperResponseLimit) {
			t.Fatal("outer boundary", n)
		}
	}
}

func TestLinuxFixedBackend(t *testing.T) {
	b := DefaultBackends()[Key{"linux", "command"}]
	if e := b.Check(); e != nil {
		t.Fatal(e)
	}
	if _, ok := DefaultBackends()[Key{"linux", "file_content"}]; ok {
		t.Fatal("unsupported backend")
	}
	for _, value := range []string{"", "日本語😀\n'\" &|$PATH"} {
		p, e := b.Prepare("h", 0, "command", map[string]any{"executable": "/bin/echo", "argv": []any{value}})
		if e != nil || b.Request(p).Command != LinuxCommandWrapper() {
			t.Fatal(e)
		}
	}
}
