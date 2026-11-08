package module

import (
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"testing"
)

func TestFileContentResponse(t *testing.T) {
	zero := 0
	for _, tc := range []struct {
		body   string
		status result.Status
		known  bool
	}{
		{`{"changed":true}`, result.Changed, true}, {`{"changed":false}`, result.OK, true},
		{`{"error":"write_failed"}`, result.Failed, true}, {`{"error":"invalid_request"}`, result.Failed, true},
		{`{"error":"execution_failed"}`, result.Failed, false}, {`{"changed":true,"changed":false}`, result.Failed, false},
		{`{"changed":null}`, result.Failed, false}, {`{"changed":true} {}`, result.Failed, false}, {`{"error":"secret"}`, result.Failed, false},
	} {
		r := transport.Response{Communication: transport.CommunicationComplete, ExitCode: &zero, Stdout: []byte(tc.body)}
		row := DecodeFileContent(r)
		if row.Status != tc.status || (row.Changed != nil) != tc.known || row.ExitCode != nil || row.StdoutBase64 != nil {
			t.Fatalf("%s: %+v", tc.body, row)
		}
		r.Communication = transport.CommunicationInterrupted
		if row := DecodeFileContent(r); row.Status != result.Failed || row.Changed != nil {
			t.Fatal(row)
		}
	}
}

func TestFileContentInvalidUTF8(t *testing.T) {
	for _, key := range []string{"path", "content"} {
		a := map[string]any{"path": `C:\file`, "content": ""}
		a[key] = "C:\\" + string([]byte{255})
		if _, err := Prepare("h", 1, "file_content", a); err == nil {
			t.Fatal(key)
		}
	}
}
