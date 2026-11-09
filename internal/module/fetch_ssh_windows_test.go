package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFetchSSH(t *testing.T)      { fetchSSH(t, "windows", "WINTABLE_SSH_FIXTURE") }
func TestLinuxFetchSSH(t *testing.T) { fetchSSH(t, "linux", "WINTABLE_LINUX_SSH_FIXTURE") }
func fetchSSH(t *testing.T, target, variable string) {
	dir := os.Getenv(variable)
	if dir == "" {
		t.Skip("requires disposable SSH fixture")
	}
	var cfg struct {
		Address, User, Container, DockerContext string
		Port                                    uint16
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
	remote := func(script string) {
		t.Helper()
		args := []string{}
		if cfg.DockerContext != "" {
			args = append(args, "--context", cfg.DockerContext)
		}
		args = append(args, "exec", cfg.Container)
		if target == "windows" {
			args = append(args, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		} else {
			args = append(args, "/usr/bin/python3", "-I", "-c", script)
		}
		out, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if e != nil {
			t.Fatalf("fixture setup failed: %v (diagnostic bytes=%d)", e, len(out))
		}
	}
	src := `C:\fixture\fetch 日本語.bin`
	if target == "linux" {
		src = "/fixture/results/fetch 日本語.bin"
	}
	dest := filepath.Join(fetchDir(t), "out")
	b := FetchBackend(target)
	host := transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")}
	run := func(p Prepared) result.TaskResult {
		t.Helper()
		s, e := (transport.SSHConnector{}).Connect(ctx, host)
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		return RunFetch(ctx, p, s, b.Command)
	}
	for _, kind := range []string{"empty", "text", "binary-over-1MiB"} {
		t.Run(kind, func(t *testing.T) {
			var expected []byte
			switch kind {
			case "text":
				expected = []byte("\xef\xbb\xbf日本語\r\nLF\n")
			case "binary-over-1MiB":
				expected = make([]byte, 1400000)
				for i := range expected {
					expected[i] = byte(i)
				}
			}
			if target == "windows" {
				script := `$b=[byte[]]::new(` + fmt.Sprint(len(expected)) + `);`
				if kind == "binary-over-1MiB" {
					script += `for($i=0;$i -lt $b.Length;$i++){$b[$i]=$i%256};`
				} else if kind == "text" {
					script += `$b=[Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString(expected) + `');`
				}
				remote(script + `[IO.File]::WriteAllBytes('C:\fixture\fetch 日本語.bin',$b)`)
			} else {
				body := "base64.b64decode('" + base64.StdEncoding.EncodeToString(expected) + "')"
				if kind == "binary-over-1MiB" {
					body = "(bytes(range(256))*5469)[:1400000]"
				}
				remote("import pathlib,base64;pathlib.Path('/fixture/results/fetch 日本語.bin').write_bytes(" + body + ")")
			}
			p, e := b.Prepare("remote", 1, "fetch", map[string]any{"src": src, "dest": dest})
			if e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 2; i++ {
				row := run(p)
				if row.Error != nil || row.Changed == nil || *row.Changed != (i == 0) {
					t.Fatalf("unexpected SSH result %+v", row)
				}
				actual, e := os.ReadFile(p.FetchDestination)
				if e != nil || !bytes.Equal(actual, expected) {
					t.Fatal("SSH byte mismatch", e)
				}
			}
		})
	}
	t.Run("product-parallel-hosts", func(t *testing.T) {
		work := fetchDir(t)
		exe := os.Getenv("WINTABLE_PRODUCT_EXE")
		if exe == "" {
			exe = os.Getenv("WINTABLE_VALIDATION_EXE")
		}
		if exe == "" {
			exe = filepath.Join(work, "wintable.exe")
			out, e := exec.CommandContext(ctx, "go", "build", "-o", exe, "../../cmd/wintable").CombinedOutput()
			if e != nil {
				t.Fatalf("product build: %v (%d diagnostic bytes)", e, len(out))
			}
		}
		write := func(name string, v any) string {
			t.Helper()
			raw, e := json.Marshal(v)
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(work, name)
			if e = os.WriteFile(path, raw, 0600); e != nil {
				t.Fatal(e)
			}
			return path
		}
		h := map[string]any{"os": target, "address": cfg.Address, "port": cfg.Port, "user": cfg.User, "private_key_file": host.PrivateKeyFile, "known_hosts_file": host.KnownHostsFile}
		inv := write("inventory.json", map[string]any{"version": 1, "hosts": map[string]any{"first": h, "second": h}})
		destination := filepath.Join(work, "out")
		tasks := write("tasks.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "fetch", "module": "fetch", "args": map[string]any{"src": src, "dest": destination}}}})
		for i := 0; i < 2; i++ {
			c := exec.CommandContext(ctx, exe, "run", "--inventory", inv, "--tasks", tasks, "--forks", "2", "--format", "json", "--include-output")
			var out, diag bytes.Buffer
			c.Stdout = &out
			c.Stderr = &diag
			if e := c.Run(); e != nil {
				t.Fatalf("product exit %v (output bytes=%d diagnostic bytes=%d)", e, out.Len(), diag.Len())
			}
			var report result.Report
			if e := json.Unmarshal(out.Bytes(), &report); e != nil || len(report.Hosts) != 2 {
				t.Fatal("invalid product result", e)
			}
			for _, host := range report.Hosts {
				if len(host.Tasks) != 1 {
					t.Fatal("missing task")
				}
				row := host.Tasks[0]
				if row.Changed == nil || *row.Changed != (i == 0) || row.Error != nil || row.StdoutBase64 != nil || row.StderrBase64 != nil {
					t.Fatal("incorrect product result", row)
				}
				p, e := b.Prepare(host.Host, 1, "fetch", map[string]any{"src": src, "dest": destination})
				if e != nil {
					t.Fatal(e)
				}
				actual, e := os.ReadFile(p.FetchDestination)
				if e != nil || len(actual) != 1400000 {
					t.Fatal("missing parallel output", e)
				}
			}
		}
	})
	t.Run("failure-preserves-existing", func(t *testing.T) {
		p, e := b.Prepare("remote", 1, "fetch", map[string]any{"src": src, "dest": dest})
		if e != nil {
			t.Fatal(e)
		}
		before, e := os.ReadFile(p.FetchDestination)
		if e != nil {
			t.Fatal(e)
		}
		for _, kind := range []string{"missing", "directory", "oversized", "symlink", "permission", "hardlink"} {
			if target == "windows" {
				script := `$p='C:\fixture\fetch 日本語.bin';Remove-Item -LiteralPath $p -Recurse -Force;`
				switch kind {
				case "directory":
					script += `New-Item -ItemType Directory -Path $p|Out-Null`
				case "oversized":
					script += `$f=[IO.File]::Create($p);$f.SetLength(1073741825);$f.Dispose()`
				case "hardlink":
					script += `[IO.File]::WriteAllText($p,'private');New-Item -ItemType HardLink -Path 'C:\fixture\fetch-hardlink' -Target $p|Out-Null`
				case "symlink":
					script += `New-Item -ItemType SymbolicLink -Path $p -Target 'C:\Windows\win.ini'|Out-Null`
				case "permission":
					script += `[IO.File]::WriteAllText($p,'private');icacls $p /deny 'wintablessh:(R)'|Out-Null`
				}
				remote(script)
			} else {
				script := "import os,shutil; p='/fixture/results/fetch 日本語.bin';shutil.rmtree(p) if os.path.isdir(p) and not os.path.islink(p) else os.unlink(p);"
				switch kind {
				case "directory":
					script += "os.mkdir(p)"
				case "oversized":
					script += "f=open(p,'wb');f.truncate(1073741825);f.close()"
				case "hardlink":
					script += "open(p,'wb').write(b'private');os.link(p,'/fixture/results/fetch-hardlink')"
				case "symlink":
					script += "os.symlink('/etc/passwd',p)"
				case "permission":
					script += "open(p,'wb').write(b'private');os.chmod(p,0)"
				}
				if kind == "missing" {
					script += "pass"
				}
				remote(script)
			}
			row := run(p)
			if row.Status != result.Failed || row.Changed == nil || *row.Changed {
				t.Fatal(kind, row)
			}
			after, e := os.ReadFile(p.FetchDestination)
			if e != nil || !bytes.Equal(after, before) {
				t.Fatal("existing content damaged", e)
			}
			if kind == "missing" {
				if target == "windows" {
					remote(`[IO.File]::WriteAllText('C:\fixture\fetch 日本語.bin','placeholder')`)
				} else {
					remote("open('/fixture/results/fetch 日本語.bin','wb').close()")
				}
			}
		}
	})
}
