package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"testing"
)

type fakeConnector struct{ calls int }

func (f *fakeConnector) Connect(context.Context, transport.Host) (transport.Session, error) {
	f.calls++
	return fakeSession{}, nil
}

type fileConnector struct{ t *testing.T }

func (f fileConnector) Connect(context.Context, transport.Host) (transport.Session, error) {
	return fileSession{f.t}, nil
}

type fileSession struct{ t *testing.T }

func (f fileSession) Close() error { return nil }
func (f fileSession) Run(_ context.Context, r transport.Request) (transport.Response, error) {
	if r.Command != module.FileContentWrapper() {
		f.t.Fatal("wrong module wrapper")
	}
	var input struct {
		Module string
		Args   map[string]string
	}
	if json.Unmarshal(r.Stdin, &input) != nil || input.Module != "file_content" || input.Args["content"] != "日本語\n" {
		f.t.Fatal("wrong stdin")
	}
	code := 0
	return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code, Stdout: []byte(`{"changed":false}`)}, nil
}
func TestBuiltInFileContent(t *testing.T) {
	validFiles(t)
	if err := os.WriteFile("t", []byte(`{"version":1,"hosts":"all","tasks":[{"name":"file","module":"file_content","args":{"path":"C:\\file.txt","content":"日本語\n"}}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	code := RunEngine(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t", "--format", "json"}, &out, &diag, engine.Runtime{Connector: fileConnector{t}})
	var report result.Report
	if code != 0 || diag.Len() != 0 || json.Unmarshal(out.Bytes(), &report) != nil || report.Counts.OK != 1 {
		t.Fatalf("%d %s %s", code, diag.String(), out.String())
	}
}

type fakeSession struct{}

func (fakeSession) Close() error { return nil }
func (fakeSession) Run(context.Context, transport.Request) (transport.Response, error) {
	code := 0
	return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code}, nil
}
func TestRunEngine(t *testing.T) {
	validFiles(t)
	f := &fakeConnector{}
	r := engine.Runtime{Connector: f, Wrappers: map[string]string{"command": "fixed"}, Decode: func(engine.TaskPlan, transport.Response) (result.Status, *bool, *result.Error) {
		b := true
		return result.Changed, &b, nil
	}}
	for _, format := range []string{"json", "text"} {
		var out, diag bytes.Buffer
		code := RunEngine(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t", "--format", format}, &out, &diag, r)
		if code != 0 || diag.Len() != 0 {
			t.Fatal(code, diag.String())
		}
		if format == "json" {
			var report result.Report
			if json.Unmarshal(out.Bytes(), &report) != nil || report.Counts.Changed != 1 {
				t.Fatal(out.String())
			}
		}
	}
	before := f.calls
	if err := os.WriteFile("t", []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	if code := RunEngine(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t"}, &out, &diag, r); code != 2 || f.calls != before {
		t.Fatal(code, f.calls)
	}
}
