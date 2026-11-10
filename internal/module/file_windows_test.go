package module

import (
	"context"
	"github.com/coco-papiyon/wintable/internal/result"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalFileWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "日本語 & space", "child")
	run := func(p string) result.TaskResult {
		t.Helper()
		b := FileBackend("windows")
		v, e := b.Prepare("h", 1, "file", map[string]any{"path": p, "state": "directory"})
		if e != nil {
			t.Fatal(e)
		}
		return RunLocalFile(context.Background(), v)
	}
	if r := run(path); r.Status != result.Changed {
		t.Fatalf("create: %+v", r)
	}
	if s, e := os.Stat(path); e != nil || !s.IsDir() {
		t.Fatal(s, e)
	}
	if r := run(path); r.Status != result.OK {
		t.Fatalf("repeat: %+v", r)
	}
	collision := filepath.Join(dir, "file")
	if e := os.WriteFile(collision, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if r := run(filepath.Join(collision, "child")); r.Status != result.Failed || r.Changed != nil {
		t.Fatalf("collision: %+v", r)
	}
	if b, _ := os.ReadFile(collision); string(b) != "keep" {
		t.Fatal("changed collision")
	}
	partial := filepath.Join(dir, "partial")
	if r := run(filepath.Join(partial, strings.Repeat("x", 300))); r.Status != result.Failed || r.Changed != nil {
		t.Fatalf("partial: %+v", r)
	}
	if s, e := os.Stat(partial); e != nil || !s.IsDir() {
		t.Fatal("parent must remain after partial failure", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := FileBackend("windows")
	p, e := b.Prepare("h", 1, "file", map[string]any{"path": filepath.Join(dir, "cancelled"), "state": "directory"})
	if e != nil {
		t.Fatal(e)
	}
	if r := RunLocalFile(ctx, p); r.Status != result.Failed || r.Changed != nil {
		t.Fatal(r)
	}
	if _, e := os.Stat(filepath.Join(dir, "cancelled")); !os.IsNotExist(e) {
		t.Fatal("cancelled request ran")
	}
	link := filepath.Join(dir, "junction")
	target := filepath.Join(dir, "target")
	os.Mkdir(target, 0700)
	// Junctions require no symlink privilege on Windows.
	c := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target)
	if e := c.Run(); e != nil {
		t.Fatal(e)
	}
	if r := run(filepath.Join(link, "forbidden")); r.Status != result.Failed {
		t.Fatalf("junction: %+v", r)
	}
	if _, e := os.Stat(filepath.Join(target, "forbidden")); !os.IsNotExist(e) {
		t.Fatal("followed junction")
	}
}
