package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/result"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFileCLIWindows(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "日本語 & space", "child")
	write := func(name string, x any) string {
		t.Helper()
		b, _ := json.Marshal(x)
		p := filepath.Join(dir, name)
		if e := os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
		return p
	}
	ip := write("inv.json", map[string]any{"version": 1, "hosts": map[string]any{"source": map[string]any{"os": "linux"}}})
	task := func(path string) any {
		return map[string]any{"name": "mkdir", "module": "file", "delegate_to": "localhost", "args": map[string]any{"path": path, "state": "directory"}}
	}
	tp := write("tasks.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{task(target)}})
	for i := 0; i < 2; i++ {
		var out, diag bytes.Buffer
		code := RunEngine(context.Background(), []string{"run", "--inventory", ip, "--tasks", tp, "--format", "json"}, &out, &diag, engine.Runtime{})
		if code != 0 {
			t.Fatal(code, diag.String(), out.String())
		}
		var r result.Report
		if e := json.Unmarshal(out.Bytes(), &r); e != nil {
			t.Fatal(e)
		}
		if got := r.Hosts[0].Tasks[0].Changed; got == nil || *got != (i == 0) {
			t.Fatal(r)
		}
	}
	marker := filepath.Join(dir, "must-not-create")
	tp = write("bad.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{task(marker), task(`C:\bad\..\path`)}})
	var out, diag bytes.Buffer
	if code := RunEngine(context.Background(), []string{"run", "--inventory", ip, "--tasks", tp}, &out, &diag, engine.Runtime{}); code == 0 {
		t.Fatal("accepted bad later task")
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("ran before preflight")
	}
}
