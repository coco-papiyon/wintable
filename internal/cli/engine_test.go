package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/engine"
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
