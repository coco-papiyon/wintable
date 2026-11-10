package module

import (
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFileSSH is part of distribution acceptance, not normal implementation checks.
func TestFileSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_SSH_FIXTURE")
	if dir == "" {
		t.Skip("Windows SSH fixture required")
	}
	var cfg struct {
		Address, User string
		Port          uint16
	}
	data, e := os.ReadFile(filepath.Join(dir, "fixture.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(data, &cfg); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s, e := (transport.SSHConnector{}).Connect(ctx, transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	b := FileBackend("windows")
	path := `C:\fixture\file-module-` + time.Now().Format("150405.000000000") + `\日本語 & space\child`
	for i := 0; i < 2; i++ {
		p, e := b.Prepare("h", 1, "file", map[string]any{"path": path, "state": "directory"})
		if e != nil {
			t.Fatal(e)
		}
		r, e := s.Run(ctx, b.Request(p))
		if e != nil {
			t.Fatal(e)
		}
		row := b.Decode(r, false)
		if row.Status == result.Failed || row.Changed == nil || *row.Changed != (i == 0) {
			t.Fatal(row)
		}
	}
	p, e := Prepare("h", 1, "command", map[string]any{"executable": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "argv": []any{"-NoProfile", "-NonInteractive", "-Command", `if (!(Test-Path -LiteralPath '` + path + `' -PathType Container)) {exit 1}`}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Run(ctx, WindowsBackends()[Key{"windows", "command"}].Request(p))
	if e != nil || DecodeCommand(r, false).Status != result.Changed {
		t.Fatal("directory observation failed", e)
	}
}
