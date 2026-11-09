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
		Address, User string
		Port          uint16
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
