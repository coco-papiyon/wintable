package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fetchRoot(t *testing.T) string {
	t.Helper()
	base, _ := filepath.Abs("../../.cache")
	os.MkdirAll(base, 0700)
	dir, e := os.MkdirTemp(base, "fe-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

type fetchConnector struct {
	connects, runs atomic.Int32
	mode           string
}

func (*fetchConnector) SupportsStream() bool { return true }
func (c *fetchConnector) Connect(context.Context, transport.Host) (transport.Session, error) {
	c.connects.Add(1)
	return &fetchSession{c: c}, nil
}

type fetchSession struct{ c *fetchConnector }

func (*fetchSession) Close() error { return nil }
func (s *fetchSession) Run(context.Context, transport.Request) (transport.Response, error) {
	s.c.runs.Add(1)
	return transport.Response{}, io.ErrUnexpectedEOF
}
func (s *fetchSession) Stream(ctx context.Context, r transport.Request, consume func(io.Reader) error) (transport.Response, error) {
	s.c.runs.Add(1)
	if s.c.mode == "timeout" {
		<-ctx.Done()
		return transport.Response{}, ctx.Err()
	}
	body := []byte("PRIVATE-FETCH-CONTENT")
	var frame bytes.Buffer
	frame.WriteString("WTF1")
	binary.Write(&frame, binary.LittleEndian, uint64(len(body)))
	frame.Write(body)
	hash := sha256.Sum256(body)
	frame.Write(hash[:])
	if s.c.mode == "corrupt" {
		frame.Truncate(frame.Len() - 1)
	}
	e := consume(&frame)
	if e != nil {
		return transport.Response{}, e
	}
	code := 0
	return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code}, nil
}
func TestFetchPreflightAndRuntime(t *testing.T) {
	root := fetchRoot(t)
	b := bundle()
	b.Tasks["tasks"] = []any{map[string]any{"name": "fetch", "module": "fetch", "args": map[string]any{"src": `C:\source`, "dest": root}}}
	p, e := Build(b, "", false)
	if e != nil {
		t.Fatal(e)
	}
	if p.Hosts[0].Tasks[0].Payload.FetchDestination == p.Hosts[1].Tasks[0].Payload.FetchDestination {
		t.Fatal("host collision")
	}
	c := &fetchConnector{}
	r := Runtime{Connector: c}
	rows, e := r.Execute(context.Background(), p, 2, time.Second, true)
	if e != nil || len(rows) != 2 {
		t.Fatal(e, rows)
	}
	for _, row := range rows {
		if row.Status != result.Changed || row.Changed == nil || !*row.Changed || row.StdoutBase64 != nil || row.StderrBase64 != nil {
			t.Fatal(row)
		}
	}
	var out, diag bytes.Buffer
	if code := result.Finish(context.Background(), &out, &diag, rows, "json", true, nil); code != 0 || strings.Contains(out.String()+diag.String(), "PRIVATE-FETCH-CONTENT") {
		t.Fatal("output leak or wrong exit")
	}
	if c.connects.Load() != 2 || c.runs.Load() != 2 {
		t.Fatal("unexpected counts")
	}
	// Last host's invalid expanded src must fail before any connection or mkdir.
	hosts := b.Inventory["hosts"].(map[string]any)
	hosts["a"].(map[string]any)["vars"] = map[string]any{"src": `C:\valid`}
	hosts["z"].(map[string]any)["vars"] = map[string]any{"src": "relative"}
	b.Tasks["tasks"].([]any)[0].(map[string]any)["args"].(map[string]any)["src"] = "${src}"
	if p, e := Build(b, "", false); p != nil || e == nil {
		t.Fatal("late invalid accepted")
	}
	// Reusing the same final path in two tasks is rejected globally.
	task := map[string]any{"name": "fetch", "module": "fetch", "args": map[string]any{"src": `C:\source`, "dest": root}}
	b.Tasks["tasks"] = []any{task, task}
	if p, e := Build(b, "", false); p != nil || e == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestFetchFailureStopsAndKeepsChangedFalse(t *testing.T) {
	for _, mode := range []string{"corrupt", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			b := bundle()
			b.Tasks["tasks"] = []any{map[string]any{"name": "fetch", "module": "fetch", "args": map[string]any{"src": `C:\source`, "dest": fetchRoot(t)}}, map[string]any{"name": "after", "module": "command", "args": map[string]any{"executable": `C:\a.exe`, "argv": []any{}}}}
			p, e := Build(b, "", false)
			if e != nil {
				t.Fatal(e)
			}
			c := &fetchConnector{mode: mode}
			rows, e := (Runtime{Connector: c}).Execute(context.Background(), p, 2, 50*time.Millisecond, true)
			if e != nil || len(rows) != 4 {
				t.Fatal(e, rows)
			}
			for i, row := range rows {
				if i%2 == 0 {
					if row.Status != result.Failed || row.Changed == nil || *row.Changed {
						t.Fatal(row)
					}
				} else if row.Status != result.Skipped {
					t.Fatal(row)
				}
			}
			if c.runs.Load() != 2 {
				t.Fatal("later task executed or retry")
			}
		})
	}
}
