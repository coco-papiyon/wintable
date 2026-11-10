package module

import (
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxFileSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_LINUX_SSH_FIXTURE")
	if dir == "" {
		t.Skip("Linux fixture required")
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	host := transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")}
	s, e := (transport.SSHConnector{}).Connect(ctx, host)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	command := func(script string) {
		t.Helper()
		p, e := PrepareLinuxCommand("h", 1, "command", map[string]any{"executable": "/usr/bin/python3", "argv": []any{"-I", "-c", script}})
		if e != nil {
			t.Fatal(e)
		}
		r, e := s.Run(ctx, DefaultBackends()[Key{"linux", "command"}].Request(p))
		if e != nil || DecodeLinuxCommand(r, true).Status != result.Changed {
			t.Fatalf("probe: %v %+v", e, r)
		}
	}
	root := "/home/runner/file-module-" + time.Now().Format("150405.000000000")
	command("import os; os.mkdir('" + root + "'); open('" + root + "/collision','w').write('keep'); os.symlink('" + root + "','" + root + "/link')")
	command("import os; os.mkdir('" + root + "/search-only'); os.mkdir('" + root + "/search-only/writable'); os.chmod('" + root + "/search-only',0o111); os.chmod('" + root + "/search-only/writable',0o300)")
	defer command("import os; os.chmod('" + root + "/search-only',0o700); os.chmod('" + root + "/search-only/writable',0o700)")
	t.Run("receiver", func(t *testing.T) {
		b := FileBackend("linux")
		for _, c := range []struct {
			p    string
			want result.Status
		}{{root + "/日本語 & space/child", result.Changed}, {root + "/日本語 & space/child", result.OK}, {root + "/collision/child", result.Failed}, {root + "/link/forbidden", result.Failed}, {"/proc/wintable-denied/child", result.Failed}} {
			p, e := b.Prepare("h", 1, "file", map[string]any{"path": c.p, "state": "directory"})
			if e != nil {
				t.Fatal(e)
			}
			r, e := s.Run(ctx, b.Request(p))
			if e != nil {
				t.Fatal(e)
			}
			if got := b.Decode(r, false); got.Status != c.want {
				t.Fatalf("%s: %+v", c.p, got)
			}
		}
		for _, input := range []string{"{}", `{"module":"file","module":"file","args":{}}`, strings.Repeat(" ", JSONLimit+1), `{"module":"file","args":{"path":"/","state":"directory"}}`} {
			r, e := s.Run(ctx, transport.Request{Command: b.Command, Stdin: []byte(input), StdoutLimit: transport.WrapperResponseLimit})
			if e != nil {
				t.Fatal(e)
			}
			got := b.Decode(r, false)
			if got.Status != result.Failed || got.Changed == nil || *got.Changed {
				t.Fatal(got)
			}
		}
		command("import os; assert os.path.isdir('" + root + "/日本語 & space/child'); assert not os.path.exists('" + root + "/forbidden'); assert open('" + root + "/collision').read()=='keep'")
	})
	t.Run("search-permissions", func(t *testing.T) {
		// The SSH user owns these directories: mode 0111 permits only search,
		// and mode 0300 permits creation without directory listing permission.
		command("import os; assert os.geteuid()!=0; os.mkdir('" + root + "/search-only/writable/baseline')")
		b := FileBackend("linux")
		for _, path := range []string{root + "/search-only/writable/baseline/child", root + "/search-only/writable/new/child"} {
			for i := 0; i < 2; i++ {
				p, err := b.Prepare("h", 1, "file", map[string]any{"path": path, "state": "directory"})
				if err != nil {
					t.Fatal(err)
				}
				r, err := s.Run(ctx, b.Request(p))
				if err != nil {
					t.Fatal(err)
				}
				got := b.Decode(r, false)
				want := result.Changed
				if i == 1 {
					want = result.OK
				}
				if got.Status != want || got.Changed == nil || *got.Changed != (i == 0) {
					t.Fatalf("%s run %d: %+v", path, i, got)
				}
			}
			command("import os; assert os.path.isdir('" + path + "')")
		}
	})
	t.Run("product", func(t *testing.T) {
		exe := os.Getenv("WINTABLE_PRODUCT_EXE")
		if exe == "" {
			t.Fatal("product exe required")
		}
		tmp := t.TempDir()
		write := func(n string, v any) string {
			t.Helper()
			b, e := json.Marshal(v)
			if e != nil {
				t.Fatal(e)
			}
			p := filepath.Join(tmp, n)
			if e = os.WriteFile(p, b, 0600); e != nil {
				t.Fatal(e)
			}
			return p
		}
		ip := write("inventory.json", map[string]any{"version": 1, "hosts": map[string]any{"linux": map[string]any{"os": "linux", "address": cfg.Address, "port": cfg.Port, "user": cfg.User, "private_key_file": host.PrivateKeyFile, "known_hosts_file": host.KnownHostsFile}}})
		tp := write("tasks.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "directory", "module": "file", "args": map[string]any{"path": root + "/product/child", "state": "directory"}}}})
		for i := 0; i < 2; i++ {
			out, e := exec.CommandContext(ctx, exe, "run", "--inventory", ip, "--tasks", tp, "--format", "json").CombinedOutput()
			if e != nil {
				t.Fatalf("%v %s", e, out)
			}
			var report result.Report
			if e = json.Unmarshal(out, &report); e != nil {
				t.Fatal(e)
			}
			row := report.Hosts[0].Tasks[0]
			if row.Changed == nil || *row.Changed != (i == 0) {
				t.Fatal(row)
			}
		}
		command("import os; assert os.path.isdir('" + root + "/product/child')")
	})
}
