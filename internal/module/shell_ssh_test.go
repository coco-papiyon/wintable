package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

func TestLinuxShellSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_LINUX_SSH_FIXTURE")
	if dir == "" {
		t.Skip("requires Linux SSH fixture")
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	host := transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")}
	session, e := (transport.SSHConnector{}).Connect(ctx, host)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	raw := func(t *testing.T, input []byte) transport.Response {
		t.Helper()
		r, e := session.Run(ctx, transport.Request{Command: LinuxShellWrapper(), Stdin: input, StdoutLimit: transport.WrapperResponseLimit})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	payload := func(script string) []byte {
		p, e := PrepareScript("h", 1, "shell", map[string]any{"script": script})
		if e != nil {
			t.Fatal(e)
		}
		return p.JSON()
	}
	t.Run("raw-invalid", func(t *testing.T) {
		for _, s := range []string{`{}`, `{"module":"command","args":{"script":"exit 0"}}`, `{"module":"shell","args":{"script":" "}}`, `{"module":"shell","args":{"script":"\u0000"}}`, `{"module":"shell","args":{"script":"\ud800"}}`, `{"module":"shell","args":{"script":"exit 0","script":"exit 0"}}`} {
			r := DecodeLinuxCommand(raw(t, []byte(s)), true)
			if r.Changed == nil || *r.Changed || r.ExitCode != nil {
				t.Fatal(r)
			}
		}
	})
	t.Run("raw-boundaries", func(t *testing.T) {
		for _, n := range []int{ContentLimit - 1, ContentLimit, ContentLimit + 1} {
			input, _ := json.Marshal(map[string]any{"module": "shell", "args": map[string]any{"script": "#" + strings.Repeat("x", n-1)}})
			r := DecodeLinuxCommand(raw(t, input), true)
			if (r.ExitCode != nil) != (n <= ContentLimit) {
				t.Fatal("script boundary", n, r)
			}
		}
		valid := payload("exit 0")
		for _, n := range []int{JSONLimit - 1, JSONLimit, JSONLimit + 1} {
			r := DecodeLinuxCommand(raw(t, append(append([]byte{}, valid...), bytes.Repeat([]byte(" "), n-len(valid))...)), true)
			if (r.ExitCode != nil) != (n <= JSONLimit) {
				t.Fatal("JSON boundary", n)
			}
		}
	})
	t.Run("output", func(t *testing.T) {
		// Output starts before the entire script is consumed: feeding must run concurrently.
		for _, n := range []int{ContentLimit - 1, ContentLimit, ContentLimit + 1} {
			script := fmt.Sprintf("head -c %d /dev/zero\nhead -c %d /dev/zero >&2\n", n, n) + strings.Repeat("# padding\n", 20000)
			r := DecodeLinuxCommand(raw(t, payload(script)), true)
			if r.ExitCode == nil || *r.ExitCode != 0 {
				t.Fatal(r)
			}
			out, _ := base64.StdEncoding.DecodeString(*r.StdoutBase64)
			diag, _ := base64.StdEncoding.DecodeString(*r.StderrBase64)
			want := min(n, ContentLimit)
			if len(out) != want || len(diag) != want || r.StdoutTruncated != (n > ContentLimit) || r.StderrTruncated != (n > ContentLimit) {
				t.Fatal("capture", n, r)
			}
		}
	})

	t.Run("early-exit", func(t *testing.T) {
		r := DecodeLinuxCommand(raw(t, payload("exit 17\n"+strings.Repeat("# comment\n", 100000))), true)
		if r.ExitCode == nil || *r.ExitCode != 17 {
			t.Fatal("early pipe closure", r)
		}
	})
	t.Run("product", func(t *testing.T) {
		product := os.Getenv("WINTABLE_PRODUCT_EXE")
		if product == "" {
			t.Fatal("missing product exe")
		}
		temp := t.TempDir()
		inv, _ := json.Marshal(map[string]any{"version": 1, "hosts": map[string]any{"h": map[string]any{"os": "linux", "address": cfg.Address, "port": cfg.Port, "user": cfg.User, "private_key_file": host.PrivateKeyFile, "known_hosts_file": host.KnownHostsFile, "vars": map[string]any{"word": "日本語"}}}})
		ip := filepath.Join(temp, "inventory.json")
		if e := os.WriteFile(ip, inv, 0600); e != nil {
			t.Fatal(e)
		}
		for _, tc := range []struct {
			script, output string
			exit           int
		}{
			{"v='${word}'; printf '%s' \"$v\" | cat", "日本語", 0},
			{"p=/tmp/wintable-shell-$$; printf data >\"$p\"; cat \"$p\"; rm \"$p\"", "data", 0},
			{"false; printf last", "last", 0},
			{"exit 19", "", 19},
			{"if (", "", 2},
		} {
			for _, include := range []bool{false, true} {
				task := map[string]any{"name": "script", "module": "shell", "args": map[string]any{"script": tc.script}}
				tasks, _ := json.Marshal(map[string]any{"version": 1, "hosts": "all", "tasks": []any{task, map[string]any{"name": "after", "module": "shell", "args": map[string]any{"script": "exit 0"}}}})
				tp := filepath.Join(temp, "tasks.json")
				if e := os.WriteFile(tp, tasks, 0600); e != nil {
					t.Fatal(e)
				}
				args := []string{"run", "--inventory", ip, "--tasks", tp, "--format", "json"}
				if include {
					args = append(args, "--include-output")
				}
				out, e := exec.CommandContext(ctx, product, args...).Output()
				exit := 0
				if e != nil {
					x, ok := e.(*exec.ExitError)
					if !ok {
						t.Fatal(e)
					}
					exit = x.ExitCode()
				}
				wantExit := 0
				if tc.exit != 0 {
					wantExit = 1
				}
				if exit != wantExit {
					t.Fatal("CLI exit", exit, e)
				}
				var report result.Report
				if e := json.Unmarshal(out, &report); e != nil {
					t.Fatal(e)
				}
				rows := report.Hosts[0].Tasks
				r := rows[0]
				if r.ExitCode == nil || *r.ExitCode != tc.exit {
					t.Fatal(r)
				}
				if tc.exit != 0 && rows[1].Status != result.Skipped {
					t.Fatal("not skipped")
				}
				if include {
					b, _ := base64.StdEncoding.DecodeString(*r.StdoutBase64)
					if string(b) != tc.output {
						t.Fatalf("output %q", b)
					}
				} else if r.StdoutBase64 != nil || r.StderrBase64 != nil {
					t.Fatal("output exposed")
				}
			}
		}
	})
	t.Run("interruption", func(t *testing.T) {
		for _, mode := range []string{"timeout", "cancel", "disconnect"} {
			t.Run(mode, func(t *testing.T) {
				s, e := (transport.SSHConnector{}).Connect(ctx, host)
				if e != nil {
					t.Fatal(e)
				}
				defer s.Close()
				c, stop := context.WithTimeout(ctx, 2*time.Second)
				defer stop()
				if mode == "cancel" {
					time.AfterFunc(300*time.Millisecond, stop)
				}
				if mode == "disconnect" {
					time.AfterFunc(300*time.Millisecond, func() { s.Close() })
				}
				r, _ := s.Run(c, transport.Request{Command: LinuxShellWrapper(), Stdin: payload("sleep 5"), StdoutLimit: transport.WrapperResponseLimit})
				if got := DecodeLinuxCommand(r, true); got.ExitCode != nil || got.Changed != nil {
					t.Fatal("uncertain execution", got)
				}
			})
		}
	})
}
