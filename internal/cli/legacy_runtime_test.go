package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

type legacyConnector struct{ response transport.Response }
type legacySession struct{ response transport.Response }

func (c legacyConnector) Connect(context.Context, transport.Host) (transport.Session, error) {
	return legacySession(c), nil
}
func (s legacySession) Close() error { return nil }
func (s legacySession) Run(context.Context, transport.Request) (transport.Response, error) {
	return s.response, nil
}

func TestLegacyRuntimeTaskAndOutput(t *testing.T) {
	validFiles(t)
	for _, full := range []bool{false, true} {
		for _, output := range []bool{false, true} {
			for _, large := range []bool{false, true} {
				stdout, stderr := []byte("hello"), []byte("err")
				if large {
					stdout = bytes.Repeat([]byte("x"), result.OutputLimit+1)
					stderr = bytes.Repeat([]byte("y"), result.OutputLimit+1)
				}
				code := 0
				r := engine.Runtime{Wrappers: map[string]string{"command": "fixed"}, Connector: legacyConnector{transport.Response{
					Communication: transport.CommunicationComplete, ExitCode: &code,
					Stdout: stdout, Stderr: stderr, StdoutTruncated: true,
				}}}
				called := false
				check := func(task engine.TaskPlan) {
					called = true
					if task.Name != "test" || task.Module != "command" || !bytes.Contains(task.Payload.JSON(), []byte("executable")) {
						t.Errorf("decoder lost task context: %+v", task)
					}
				}
				r.Decode = func(task engine.TaskPlan, _ transport.Response) (result.Status, *bool, *result.Error) {
					check(task)
					return result.OK, nil, nil
				}
				if full {
					r.DecodeResult = func(task engine.TaskPlan, _ transport.Response, gotOutput bool) result.TaskResult {
						check(task)
						if gotOutput != output {
							t.Error("incorrect output flag")
						}
						// The full decoder owns its output; raw transport bytes must not replace it.
						return result.TaskResult{Status: result.OK, ExitCode: &code}
					}
				}
				args := []string{"run", "--inventory", "i", "--tasks", "t", "--format", "json"}
				if output {
					args = append(args, "--include-output")
				}
				var out, diag bytes.Buffer
				if got := RunEngine(context.Background(), args, &out, &diag, r); got != 0 || !called {
					t.Fatalf("exit=%d called=%v diagnostics=%s", got, called, diag.String())
				}
				var report result.Report
				if err := json.Unmarshal(out.Bytes(), &report); err != nil {
					t.Fatal(err)
				}
				row := report.Hosts[0].Tasks[0]
				if row.ExitCode == nil || *row.ExitCode != code {
					t.Error("exit code lost")
				}
				if !output || full {
					if row.StdoutBase64 != nil || row.StderrBase64 != nil || row.StdoutTruncated || row.StderrTruncated {
						t.Error("unexpected capture")
					}
					continue
				}
				for _, stream := range []struct {
					got  *string
					want []byte
				}{{row.StdoutBase64, stdout}, {row.StderrBase64, stderr}} {
					if stream.got == nil {
						t.Fatal("missing output")
					}
					decoded, err := base64.StdEncoding.DecodeString(*stream.got)
					if err != nil || !bytes.Equal(decoded, stream.want[:min(len(stream.want), result.OutputLimit)]) {
						t.Error("incorrect captured output")
					}
				}
				if !row.StdoutTruncated || row.StderrTruncated != large {
					t.Error("incorrect truncation flags")
				}
			}
		}
	}
}
