package module

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/transport"
)

// Distribution-only Windows SSH check; ordinary runs use TestPowerShellWindows.
func TestPowerShellSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_SSH_FIXTURE")
	if dir == "" {
		t.Skip("requires Windows SSH fixture")
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s, e := (transport.SSHConnector{}).Connect(ctx, transport.Host{Address: cfg.Address, User: cfg.User, Port: cfg.Port, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, tc := range []struct {
		script string
		exit   int
	}{
		{"[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); [Console]::Write('日本語')", 0},
		{"exit 23", 23}, {"throw 'failure'", 1}, {"if (", 1},
		{"$s='x'*1048577;[Console]::Out.Write($s);[Console]::Error.Write($s)", 0},
	} {
		p, e := PrepareScript("h", 1, "powershell", map[string]any{"script": tc.script})
		if e != nil {
			t.Fatal(e)
		}
		raw, e := s.Run(ctx, transport.Request{Command: PowerShellWrapper(), Stdin: p.JSON(), StdoutLimit: transport.WrapperResponseLimit})
		if e != nil {
			t.Fatal(e)
		}
		r := DecodeCommand(raw, true)
		if r.ExitCode == nil || *r.ExitCode != tc.exit {
			t.Fatal(r)
		}
		if tc.script[0] == '[' {
			out, _ := base64.StdEncoding.DecodeString(*r.StdoutBase64)
			if string(out) != "日本語" {
				t.Fatal("UTF8")
			}
		}
		if tc.script[0] == '$' && (!r.StdoutTruncated || !r.StderrTruncated) {
			t.Fatal("capture")
		}
	}
}
