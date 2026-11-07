package engine

import (
	"context"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

func TestCommandResultBoundary(t *testing.T) {
	for _, output := range []bool{false, true} {
		p, r := fixture(t, 1)
		r.Decode = nil
		r.DecodeResult = func(_ TaskPlan, response transport.Response, output bool) result.TaskResult {
			return module.DecodeCommand(response, output)
		}
		r.Connector = connectorFunc(func(context.Context, transport.Host) (transport.Session, error) {
			return &session{run: func(context.Context, transport.Request) (transport.Response, error) {
				outer := 0
				return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &outer, Stdout: []byte(`{"exit_code":17,"stdout":"AP8=","stderr":"","stdout_truncated":false,"stderr_truncated":false}`)}, nil
			}, close: func() error { return nil }}, nil
		})
		rows, err := r.Execute(context.Background(), p, 1, time.Second, output)
		if err != nil || len(rows) != 3 || rows[0].ExitCode == nil || *rows[0].ExitCode != 17 || rows[0].Changed != nil || rows[0].Status != result.Failed || rows[1].Status != result.Skipped || rows[0].Host != "h000" || rows[0].Task != "same" {
			t.Fatal(rows, err)
		}
		if output {
			if rows[0].StdoutBase64 == nil || *rows[0].StdoutBase64 != "AP8=" {
				t.Fatal("outer JSON exposed", rows[0])
			}
		} else if rows[0].StdoutBase64 != nil {
			t.Fatal("output exposed")
		}
	}
}
