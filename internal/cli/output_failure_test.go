package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/coco-papiyon/wintable/internal/result"
)

// probeWriter records attempted bytes even when the destination rejects them.
type probeWriter struct {
	bytes.Buffer
	calls  int
	mode   string
	cancel context.CancelFunc
}

func (w *probeWriter) Write(p []byte) (int, error) {
	w.calls++
	w.Buffer.Write(p)
	if w.cancel != nil {
		w.cancel()
	}
	switch w.mode {
	case "error":
		return 0, errors.New("PRIVATE writer failure")
	case "short":
		return len(p) - 1, nil
	default:
		return len(p), nil
	}
}

func TestRunResultsOutputFailurePriority(t *testing.T) {
	validFiles(t)
	for _, format := range []string{"json", "text"} {
		for _, outputMode := range []string{"error", "short"} {
			for _, diagnosticMode := range []string{"ok", "error", "short"} {
				for _, cancelAt := range []string{"none", "stdout", "stderr"} {
					t.Run(format+"/"+outputMode+"/"+diagnosticMode+"/"+cancelAt, func(t *testing.T) {
						ctx, cancel := context.WithCancel(context.Background())
						defer cancel()
						out := &probeWriter{mode: outputMode}
						diag := &probeWriter{mode: diagnosticMode}
						if cancelAt == "stdout" {
							out.cancel = cancel
						}
						if cancelAt == "stderr" {
							diag.cancel = cancel
						}
						calls := 0
						code := RunResults(ctx, []string{"run", "--inventory", "i", "--tasks", "t", "--format", format}, out, diag, func(context.Context, Options) ([]result.TaskResult, error) {
							calls++
							return []result.TaskResult{{Host: "win", Status: result.Failed}}, nil
						})
						want := 3
						if cancelAt != "none" {
							want = 130
						}
						if code != want || calls != 1 || out.calls != 1 || diag.calls != 1 {
							t.Fatalf("code=%d want=%d executor=%d stdout=%d stderr=%d", code, want, calls, out.calls, diag.calls)
						}
						if diag.String() != "output_failed\n" || bytes.Contains(out.Bytes(), []byte("output_failed")) || bytes.Contains(out.Bytes(), []byte("PRIVATE")) {
							t.Fatalf("unsafe or misplaced diagnostic: stdout=%q stderr=%q", out.String(), diag.String())
						}
					})
				}
			}
		}
	}
}
