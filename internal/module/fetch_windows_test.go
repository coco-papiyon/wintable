package module

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type localFetchSession struct {
	calls   int
	command string
}

func (*localFetchSession) Close() error { return nil }
func (*localFetchSession) Run(context.Context, transport.Request) (transport.Response, error) {
	panic("fetch must use Stream")
}
func (s *localFetchSession) Stream(ctx context.Context, req transport.Request, consume func(io.Reader) error) (transport.Response, error) {
	s.calls++
	s.command = req.Command
	c := exec.CommandContext(ctx, "cmd.exe")
	c.SysProcAttr = rawCommand(req.Command)
	c.Stdin = bytes.NewReader(req.Stdin)
	c.Stderr = io.Discard
	pipe, e := c.StdoutPipe()
	if e != nil {
		return transport.Response{}, e
	}
	if e = c.Start(); e != nil {
		return transport.Response{}, e
	}
	readErr := consume(pipe)
	if readErr != nil {
		c.Process.Kill()
	}
	e = c.Wait()
	if readErr != nil {
		return transport.Response{}, readErr
	}
	if e != nil {
		return transport.Response{}, e
	}
	zero := 0
	return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &zero}, nil
}
func fetchDir(t *testing.T) string {
	t.Helper()
	base, e := filepath.Abs("../../.cache")
	if e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(base, 0700)
	dir, e := os.MkdirTemp(base, "fm-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
func TestFetchWindows(t *testing.T) {
	dir := fetchDir(t)
	src := filepath.Join(dir, "日本語 空白.bin")
	dest := filepath.Join(dir, "out")
	b := FetchBackend("windows")
	s := &localFetchSession{}
	for _, body := range [][]byte{{}, []byte("日本語\r\n\xef\xbb\xbf"), bytes.Repeat([]byte{0, 255, 13, 10}, 400000)} {
		if e := os.WriteFile(src, body, 0600); e != nil {
			t.Fatal(e)
		}
		p, e := b.Prepare("host", 1, "fetch", map[string]any{"src": src, "dest": dest})
		if e != nil {
			t.Fatal(e)
		}
		if bytes.Contains(p.JSON(), []byte("dest")) {
			t.Fatal("local destination sent remotely")
		}
		for i := 0; i < 2; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			row := RunFetch(ctx, p, s, b.Command)
			cancel()
			if row.Error != nil || row.Changed == nil || *row.Changed != (i == 0) {
				t.Fatalf("bad result %+v", row)
			}
			data, e := os.ReadFile(p.FetchDestination)
			if e != nil || !bytes.Equal(data, body) {
				t.Fatal("content mismatch", e)
			}
			encoded, _ := json.Marshal(row)
			if row.StdoutBase64 != nil || row.StderrBase64 != nil || row.ExitCode != nil || bytes.Contains(encoded, []byte("日本語")) {
				t.Fatal("data exposed")
			}
		}
	}
	for _, path := range []string{dir, filepath.Join(dir, "missing")} {
		p, e := b.Prepare("host", 1, "fetch", map[string]any{"src": path, "dest": dest})
		if e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		row := RunFetch(ctx, p, s, b.Command)
		cancel()
		if row.Status != result.Failed || row.Changed == nil || *row.Changed {
			t.Fatal("invalid source accepted", row)
		}
	}
}
