package module

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxFileContent(t *testing.T) {
	for _, p := range []string{"", "/", "relative", "/a/", "/a//b", "/a/../b", "/a/./b", "/a\n", "/a\x00", "/\xff"} {
		if _, e := PrepareLinuxFileContent("h", 1, "file_content", map[string]any{"path": p, "content": ""}); e == nil {
			t.Fatal(p)
		}
	}
	for _, n := range []int{ContentLimit - 1, ContentLimit, ContentLimit + 1} {
		_, e := PrepareLinuxFileContent("h", 1, "file_content", map[string]any{"path": "/a", "content": strings.Repeat("a", n)})
		if (e == nil) != (n <= ContentLimit) {
			t.Fatal(n, e)
		}
	}
	zero := 0
	for _, s := range []string{`{"changed":false}`, `{"changed":true}`, `{"error":"write_failed"}`, `{"error":"execution_failed"}`, `{"error":"invalid_request"}`, `{"changed":null}`, `{"changed":true,"changed":false}`, `{"error":"start_failed"}`, `{"changed":true}{}`} {
		r := DecodeLinuxFileContent(transport.Response{Communication: transport.CommunicationComplete, ExitCode: &zero, Stdout: []byte(s)}, true)
		known := s == `{"changed":false}` || s == `{"changed":true}` || s == `{"error":"write_failed"}` || s == `{"error":"invalid_request"}`
		if (r.Changed != nil) != known || r.ExitCode != nil || r.StdoutBase64 != nil {
			t.Fatal(s, r)
		}
	}
}

func TestLinuxFileContentSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_LINUX_SSH_FIXTURE")
	if dir == "" {
		t.Skip("requires Linux SSH fixture")
	}
	var cfg struct {
		Address, User, Container, DockerContext string
		Port                                    uint16
	}
	b, e := os.ReadFile(filepath.Join(dir, "fixture.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &cfg); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	host := transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")}
	session, e := (transport.SSHConnector{}).Connect(ctx, host)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	t.Run("distribution-example", func(t *testing.T) {
		packageDir := filepath.Dir(os.Getenv("WINTABLE_PRODUCT_EXE"))
		temp := t.TempDir()
		inventory, err := os.ReadFile(filepath.Join(packageDir, "examples", "linux-inventory.json"))
		if err != nil {
			t.Fatal(err)
		}
		var inv map[string]any
		if err = json.Unmarshal(inventory, &inv); err != nil {
			t.Fatal(err)
		}
		h := inv["hosts"].(map[string]any)["linux01"].(map[string]any)
		h["address"], h["port"], h["user"] = cfg.Address, cfg.Port, cfg.User
		h["private_key_file"], h["known_hosts_file"] = host.PrivateKeyFile, host.KnownHostsFile
		inventory, _ = json.Marshal(inv)
		ip := filepath.Join(temp, "inventory.json")
		if err = os.WriteFile(ip, inventory, 0600); err != nil {
			t.Fatal(err)
		}
		var previous string
		for i, name := range []string{"linux-file-content.json", "linux-file-content.json", "linux-file-content-diff.json"} {
			tp := filepath.Join(packageDir, "examples", name)
			data, err := os.ReadFile(tp)
			if err != nil {
				t.Fatal(err)
			}
			var task struct {
				Tasks []struct {
					Args struct{ Path, Content string }
				}
			}
			if err = json.Unmarshal(data, &task); err != nil || len(task.Tasks) != 1 {
				t.Fatal("invalid distribution example", err)
			}
			out, err := exec.CommandContext(ctx, os.Getenv("WINTABLE_PRODUCT_EXE"), "run", "--inventory", ip, "--tasks", tp, "--format", "json").CombinedOutput()
			if err != nil {
				t.Fatalf("example exit: %v %s", err, out)
			}
			var report result.Report
			if err = json.Unmarshal(out, &report); err != nil || len(report.Hosts) != 1 || len(report.Hosts[0].Tasks) != 1 {
				t.Fatal("example result", err)
			}
			row := report.Hosts[0].Tasks[0]
			wantStatus := "changed"
			if i == 1 {
				wantStatus = "ok"
			}
			if row.Changed == nil || *row.Changed != (i != 1) || string(row.Status) != wantStatus {
				t.Fatal(row)
			}
			args := task.Tasks[0].Args
			probe := fmt.Sprintf(`import os,stat,hashlib,json
p=%q
s=os.stat(p)
assert hashlib.sha256(open(p,'rb').read()).hexdigest()==%q
assert (s.st_uid,s.st_gid,stat.S_IMODE(s.st_mode))==(os.geteuid(),os.getegid(),0o600)
assert not [x for x in os.listdir(os.path.dirname(p)) if x.startswith('.wintable-')]
print(json.dumps([s.st_uid,s.st_gid,stat.S_IMODE(s.st_mode),s.st_mtime_ns]))
`, args.Path, fmt.Sprintf("%x", sha256.Sum256([]byte(args.Content))))
			r, err := session.Run(ctx, transport.Request{Command: "/usr/bin/python3 -I -", Stdin: []byte(probe), StdoutLimit: 4096})
			if err != nil || r.ExitCode == nil || *r.ExitCode != 0 {
				t.Fatalf("example probe: %v %s", err, r.Stderr)
			}
			if i == 1 && string(r.Stdout) != previous {
				t.Fatal("same-content example modified attributes/mtime")
			}
			previous = string(r.Stdout)
		}
	})
	t.Run("commit-disconnect", func(t *testing.T) { testFileCommitDisconnect(t, ctx, host, session) })
	// Root is used only to provision ownership impossible for the SSH runner.
	if !strings.HasPrefix(cfg.Container, "wintable-linux-") || cfg.DockerContext == "" {
		t.Fatal("unsafe fixture")
	}
	setup := exec.CommandContext(ctx, "docker", "--context", cfg.DockerContext, "exec", "-i", cfg.Container, "/usr/bin/python3", "-I", "-")
	setup.Stdin = strings.NewReader(`import os
p='/home/runner/foreign-file'
with open(p,'wb') as f: f.write(b'foreign')
os.chown(p,0,0)
os.chmod(p,0o644)
for sg in (0,1):
 for acl in (0,1):
  p=f'/home/runner/attrs-{sg}-{acl}'
  os.mkdir(p)
  os.chown(p,1000,0)
  os.chmod(p,0o2700 if sg else 0o700)
`)
	if out, err := setup.CombinedOutput(); err != nil {
		t.Fatalf("fixture setup: %v %s", err, out)
	}
	t.Run("raw-boundaries", func(t *testing.T) {
		for _, n := range []int{ContentLimit - 1, ContentLimit, ContentLimit + 1} {
			b, _ := json.Marshal(map[string]any{"module": "file_content", "args": map[string]any{"path": "/missing-parent/file", "content": strings.Repeat("x", n)}})
			r, e := session.Run(ctx, transport.Request{Command: LinuxFileContentWrapper(), Stdin: b, StdoutLimit: 4096})
			want := `{"error":"write_failed"}`
			if n > ContentLimit {
				want = `{"error":"invalid_request"}`
			}
			if e != nil || string(r.Stdout) != want {
				t.Fatalf("content boundary %d %v %s", n, e, r.Stdout)
			}
		}
		valid := `{"module":"file_content","args":{"path":"/missing-parent/file","content":""}}`
		for _, n := range []int{JSONLimit - 1, JSONLimit, JSONLimit + 1} {
			r, e := session.Run(ctx, transport.Request{Command: LinuxFileContentWrapper(), Stdin: []byte(valid + strings.Repeat(" ", n-len(valid))), StdoutLimit: 4096})
			want := `{"error":"write_failed"}`
			if n > JSONLimit {
				want = `{"error":"invalid_request"}`
			}
			if e != nil || string(r.Stdout) != want {
				t.Fatalf("JSON boundary %d %v %s", n, e, r.Stdout)
			}
		}
		for _, s := range []string{"", "\xef\xbb\xbf" + valid, "\xff", valid + "{}", strings.Replace(valid, `"content":""`, `"content":"\ud800"`, 1), strings.Replace(valid, `"content":""`, `"content":"","content":""`, 1), strings.Replace(valid, `"path":`, `"unknown":0,"path":`, 1)} {
			r, e := session.Run(ctx, transport.Request{Command: LinuxFileContentWrapper(), Stdin: []byte(s), StdoutLimit: 4096})
			if e != nil || string(r.Stdout) != `{"error":"invalid_request"}` {
				t.Fatalf("invalid input %v %s", e, r.Stdout)
			}
		}
	})
	t.Run("API-cases", func(t *testing.T) {
		code, e := os.ReadFile("../../tests/linux-ssh/file_content_cases.py")
		if e != nil {
			t.Fatal(e)
		}
		payload, _ := json.Marshal(linuxFileContentSource)
		script := string(code) + "\ncases(" + string(payload) + ")\n"
		r, e := session.Run(ctx, transport.Request{Command: "/usr/bin/python3 -I -", Stdin: []byte(script), StdoutLimit: transport.WrapperResponseLimit})
		if e != nil || r.ExitCode == nil || *r.ExitCode != 0 || !strings.Contains(string(r.Stdout), "cases passed") {
			t.Fatalf("%v %+v %s", e, r, r.Stderr)
		}
		for _, name := range []string{"foreign-owner", "setgid-0-acl-0", "setgid-0-acl-1", "setgid-1-acl-0", "setgid-1-acl-1", "bytes-equal", "mode-retention", "type-ancestor-rejections", "acl-xattr-permissions", "target-link-swap", "new-destination-race", "commit-cleanup-fsync", "ancestor-rename", "temp-substitution"} {
			t.Run(name, func(t *testing.T) {
				if strings.Count(string(r.Stdout), "PASS "+name+"\n") != 1 {
					t.Fatal("missing/duplicate API assertion group", name)
				}
			})
		}
		t.Log(string(r.Stdout))
	})
	t.Run("product-bytes", func(t *testing.T) {
		temp := t.TempDir()
		inv := map[string]any{"version": 1, "hosts": map[string]any{"linux": map[string]any{"os": "linux", "address": cfg.Address, "port": cfg.Port, "user": cfg.User, "private_key_file": host.PrivateKeyFile, "known_hosts_file": host.KnownHostsFile}}}
		b, _ := json.Marshal(inv)
		ip := filepath.Join(temp, "inventory.json")
		os.WriteFile(ip, b, 0600)
		for i, content := range []string{"", "", "日本語😀\r\n\x00\n", strings.Repeat("x", ContentLimit)} {
			tasks := map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "write", "module": "file_content", "args": map[string]any{"path": "/home/runner/product-file", "content": content}}}}
			b, _ := json.Marshal(tasks)
			tp := filepath.Join(temp, "tasks.json")
			os.WriteFile(tp, b, 0600)
			out, e := exec.CommandContext(ctx, os.Getenv("WINTABLE_PRODUCT_EXE"), "run", "--inventory", ip, "--tasks", tp, "--format", "json").CombinedOutput()
			if e != nil {
				t.Fatalf("%v %s", e, out)
			}
			var report result.Report
			if e = json.Unmarshal(out, &report); e != nil {
				t.Fatal(e)
			}
			row := report.Hosts[0].Tasks[0]
			if row.Changed == nil || *row.Changed != (i != 1) {
				t.Fatal(row)
			}
			probe := "import os,sys,hashlib,json; p='/home/runner/product-file'; s=os.stat(p); print(json.dumps([hashlib.sha256(open(p,'rb').read()).hexdigest(),s.st_mode&4095,s.st_uid==os.geteuid()]))"
			r, e := session.Run(ctx, transport.Request{Command: "/usr/bin/python3 -I -c '" + strings.ReplaceAll(probe, "'", "'\"'\"'") + "'", StdoutLimit: 4096})
			expected := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
			if e != nil || !strings.Contains(string(r.Stdout), expected) || !strings.Contains(string(r.Stdout), ", 384, true]") {
				t.Fatalf("probe %v %s %s", e, r.Stdout, r.Stderr)
			}
		}
	})
}
