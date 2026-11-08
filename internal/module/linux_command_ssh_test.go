package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

// TestLinuxCommandSSH separates product-exe cases from raw receiver injection.
func TestLinuxCommandSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_LINUX_SSH_FIXTURE")
	if dir == "" {
		t.Skip("run Linux fixture with product.ps1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	host := transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")}
	session, e := (transport.SSHConnector{}).Connect(ctx, host)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	request := func(exe string, args []any) []byte {
		p, e := PrepareLinuxCommand("h", 0, "command", map[string]any{"executable": exe, "argv": args})
		if e != nil {
			t.Fatal(e)
		}
		return p.JSON()
	}
	raw := func(line string, input []byte) result.TaskResult {
		r, e := session.Run(ctx, transport.Request{Command: line, Stdin: input, StdoutLimit: transport.WrapperResponseLimit})
		if e != nil {
			t.Fatal(e)
		}
		return DecodeLinuxCommand(r, true)
	}
	t.Run("raw-invalid", func(t *testing.T) {
		valid := string(request("/usr/bin/touch", []any{"/fixture/results/forbidden-child"}))
		bad := []string{"", "\xef\xbb\xbf" + valid, "\xff", valid + "{}", strings.Replace(valid, `"module":"command"`, `"module":"command","module":"command"`, 1), strings.Replace(valid, `"args":`, `"unknown":0,"args":`, 1), strings.Replace(valid, `"argv":["/fixture/results/forbidden-child"]`, `"argv":null`, 1), strings.Replace(valid, `"argv":["/fixture/results/forbidden-child"]`, `"argv":["\ud800"]`, 1), strings.Replace(valid, `"argv":["/fixture/results/forbidden-child"]`, `"argv":["\u0000"]`, 1), strings.Repeat("[", 10000), strings.Repeat(" ", JSONLimit+1)}
		// Shape mutations below also cover requests whose original argv is nonempty.
		bad = append(bad, strings.Replace(valid, `"argv":["/fixture/results/forbidden-child"]`, `"argv":null`, 1), strings.Replace(valid, `"executable":"/usr/bin/touch"`, `"executable":"relative"`, 1))
		for i, s := range bad {
			r := raw(LinuxCommandWrapper(), []byte(s))
			if r.Changed == nil || *r.Changed || r.ExitCode != nil {
				t.Fatal(i, r)
			}
		}
	})
	t.Run("raw-boundaries", func(t *testing.T) {
		for _, n := range []int{32767, 32768, 32769} {
			input, _ := json.Marshal(map[string]any{"module": "command", "args": map[string]any{"executable": "/" + strings.Repeat("x", n-1), "argv": []string{}}})
			r, e := session.Run(ctx, transport.Request{Command: LinuxCommandWrapper(), Stdin: input, StdoutLimit: transport.WrapperResponseLimit})
			if e != nil {
				t.Fatal(e)
			}
			want := `{"error":"start_failed"}`
			if n > 32768 {
				want = `{"error":"invalid_request"}`
			}
			if string(r.Stdout) != want {
				t.Fatal("exe boundary", n)
			}
		}
		for _, n := range []int{32767, 32768, 32769} {
			input, _ := json.Marshal(map[string]any{"module": "command", "args": map[string]any{"executable": "/bin/true", "argv": []string{strings.Repeat("x", n)}}})
			r := raw(LinuxCommandWrapper(), input)
			if (r.ExitCode != nil) != (n <= 32768) {
				t.Fatal("arg", n, r)
			}
		}
		for _, n := range []int{4095, 4096, 4097} {
			a := make([]string, n)
			input, _ := json.Marshal(map[string]any{"module": "command", "args": map[string]any{"executable": "/bin/true", "argv": a}})
			r := raw(LinuxCommandWrapper(), input)
			if (r.ExitCode != nil) != (n <= 4096) {
				t.Fatal("argv", n, r)
			}
		}
		for _, n := range []int{65535, 65536, 65537} {
			a := []string{strings.Repeat("a", 32768), strings.Repeat("b", n-len("/bin/true")-3-32768)}
			input, _ := json.Marshal(map[string]any{"module": "command", "args": map[string]any{"executable": "/bin/true", "argv": a}})
			r := raw(LinuxCommandWrapper(), input)
			if (r.ExitCode != nil) != (n <= 65536) {
				t.Fatal("total", n, r)
			}
		}
		input := request("/bin/true", []any{})
		for _, n := range []int{JSONLimit - 1, JSONLimit, JSONLimit + 1} {
			r := raw(LinuxCommandWrapper(), append(append([]byte{}, input...), bytes.Repeat([]byte(" "), n-len(input))...))
			if (r.ExitCode != nil) != (n <= JSONLimit) {
				t.Fatal("request", n, r)
			}
		}
	})
	t.Run("raw-EOF", func(t *testing.T) {
		key, e := os.ReadFile(host.PrivateKeyFile)
		if e != nil {
			t.Fatal(e)
		}
		signer, e := ssh.ParsePrivateKey(key)
		if e != nil {
			t.Fatal(e)
		}
		verify, e := knownhosts.New(host.KnownHostsFile)
		if e != nil {
			t.Fatal(e)
		}
		client, e := ssh.Dial("tcp", net.JoinHostPort(host.Address, fmt.Sprint(host.Port)), &ssh.ClientConfig{User: host.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: verify, Timeout: 5 * time.Second})
		if e != nil {
			t.Fatal(e)
		}
		defer client.Close()
		s, e := client.NewSession()
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		pipe, e := s.StdinPipe()
		if e != nil {
			t.Fatal(e)
		}
		var out, diag bytes.Buffer
		s.Stdout = &out
		s.Stderr = &diag
		if e = s.Start(LinuxCommandWrapper()); e != nil {
			t.Fatal(e)
		}
		input := request("/usr/bin/touch", []any{"/fixture/results/eof-child"})
		for len(input) > 0 {
			n := 7
			if n > len(input) {
				n = len(input)
			}
			if _, e = pipe.Write(input[:n]); e != nil {
				t.Fatal(e)
			}
			input = input[n:]
		}
		done := make(chan error, 1)
		go func() { done <- s.Wait() }()
		select {
		case <-done:
			t.Fatal("receiver executed before EOF")
		case <-time.After(150 * time.Millisecond):
		}
		r := raw(LinuxCommandWrapper(), request("/usr/bin/test", []any{"!", "-e", "/fixture/results/eof-child"}))
		if r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatal("pre-EOF child marker")
		}
		pipe.Close()
		select {
		case e = <-done:
			if e != nil || diag.Len() != 0 {
				t.Fatal("EOF response", e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("EOF stalled")
		}
		zero := 0
		r = DecodeLinuxCommand(transport.Response{Communication: transport.CommunicationComplete, ExitCode: &zero, Stdout: out.Bytes()}, false)
		if r.Changed == nil || !*r.Changed {
			t.Fatal(r)
		}
		r = raw(LinuxCommandWrapper(), request("/usr/bin/test", []any{"-e", "/fixture/results/eof-child"}))
		if r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatal("post-EOF child marker")
		}
	})
	t.Run("raw-runtime", func(t *testing.T) {
		input := request("/usr/bin/touch", []any{"/fixture/results/forbidden-child"})
		for _, prefix := range []string{"import sys;sys.platform='other'\n", "import sys;sys.version_info=(3,12)\n", "import platform;platform.python_implementation=lambda:'Other'\n"} {
			line := "/usr/bin/python3 -I -c '" + strings.ReplaceAll(prefix+linuxCommandSource, "'", "'\"'\"'") + "'"
			r := raw(line, input)
			if r.Changed == nil || *r.Changed {
				t.Fatal(r)
			}
		}
		r := raw("/missing/runtime -I -c 'pass'", input)
		if r.Changed != nil {
			t.Fatal("runtime absence", r)
		}
		r = raw(LinuxCommandWrapper(), request("/usr/bin/test", []any{"!", "-e", "/fixture/results/forbidden-child"}))
		if r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatal("child started")
		}
	})
	t.Run("raw-E2BIG", func(t *testing.T) {
		prefix := "import os,subprocess\nfor i in range(4): os.environ['WINTABLE_TEST_'+str(i)]='e'*30000\nreal_popen=subprocess.Popen\ndef observe(*a,**kw):\n try: return real_popen(*a,**kw)\n except OSError as e:\n  open('/fixture/results/e2big-errno','w').write(str(e.errno))\n  raise\nsubprocess.Popen=observe\n"
		line := "/usr/bin/python3 -I -c '" + strings.ReplaceAll(prefix+linuxCommandSource, "'", "'\"'\"'") + "'"
		// Linux's 128KiB ARG_MAX floor with a 512KiB stack is exceeded by
		// inherited environment plus this otherwise valid 32KiB argument.
		r := raw("ulimit -s 512; "+line, request("/bin/true", []any{strings.Repeat("a", 32768)}))
		if r.Changed == nil || *r.Changed || r.ExitCode != nil {
			t.Fatal(r)
		}
		r = raw(LinuxCommandWrapper(), request("/bin/cat", []any{"/fixture/results/e2big-errno"}))
		if r.StdoutBase64 == nil || *r.StdoutBase64 != base64.StdEncoding.EncodeToString([]byte("7")) {
			t.Fatal("E2BIG errno not observed")
		}
	})
	product := os.Getenv("WINTABLE_PRODUCT_EXE")
	if product == "" {
		t.Fatal("missing Windows product exe")
	}
	inv := map[string]any{"version": 1, "hosts": map[string]any{"linux": map[string]any{"os": "linux", "address": cfg.Address, "port": cfg.Port, "user": cfg.User, "private_key_file": host.PrivateKeyFile, "known_hosts_file": host.KnownHostsFile}}}
	temp := t.TempDir()
	inventory := filepath.Join(temp, "inventory.json")
	data, _ = json.Marshal(inv)
	os.WriteFile(inventory, data, 0600)
	run := func(t *testing.T, exe string, argv []any, include bool, wantExit int, extra ...string) result.TaskResult {
		t.Helper()
		tasks := map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "probe", "module": "command", "args": map[string]any{"executable": exe, "argv": argv}}, map[string]any{"name": "after", "module": "command", "args": map[string]any{"executable": "/bin/true", "argv": []any{}}}}}
		path := filepath.Join(temp, "tasks.json")
		data, _ := json.Marshal(tasks)
		os.WriteFile(path, data, 0600)
		args := []string{"run", "--inventory", inventory, "--tasks", path, "--format", "json"}
		if include {
			args = append(args, "--include-output")
		}
		args = append(args, extra...)
		command := exec.CommandContext(ctx, product, args...)
		var diag bytes.Buffer
		command.Stderr = &diag
		out, e := command.Output()
		code := 0
		if e != nil {
			if exit, ok := e.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				t.Fatal(e)
			}
		}
		if code != wantExit {
			t.Fatalf("CLI exit %d want %d diag=%s", code, wantExit, diag.String())
		}
		var report result.Report
		if e = json.Unmarshal(out, &report); e != nil {
			t.Fatalf("JSON: %v %s", e, out)
		}
		rows := report.Hosts[0].Tasks
		if len(rows) != 2 || (wantExit == 1 && rows[1].Status != result.Skipped) {
			t.Fatal(rows)
		}
		return rows[0]
	}
	t.Run("product-argv", func(t *testing.T) {
		for _, a := range [][]any{{}, {"", "日本語😀", "a'b\"c", "a b", "a\nb", "&|;$PATH"}} {
			// Independent Python probe reports lengths and hex, avoiding the receiver's serializer.
			code := "import sys,json;print(json.dumps([[len(x.encode()),x.encode().hex()] for x in sys.argv[1:]]))"
			argv := append([]any{"-I", "-c", code}, a...)
			r := run(t, "/usr/bin/python3", argv, true, 0)
			out, _ := base64.StdEncoding.DecodeString(*r.StdoutBase64)
			var observed [][]any
			json.Unmarshal(out, &observed)
			expected := [][]any{}
			for _, v := range a {
				s := v.(string)
				expected = append(expected, []any{float64(len(s)), fmt.Sprintf("%x", []byte(s))})
			}
			if !reflect.DeepEqual(observed, expected) {
				t.Fatal("argv byte mismatch", observed, expected)
			}
		}
	})
	t.Run("product-output", func(t *testing.T) {
		for _, n := range []int{1048575, 1048576, 1048577} {
			code := fmt.Sprintf("import os,threading;f=lambda fd,b:os.write(fd,b*%d);t=threading.Thread(target=f,args=(2,b'\\xfe'));t.start();f(1,b'\\xff');t.join()", n)
			r := run(t, "/usr/bin/python3", []any{"-I", "-c", code}, true, 0)
			a, _ := base64.StdEncoding.DecodeString(*r.StdoutBase64)
			b, _ := base64.StdEncoding.DecodeString(*r.StderrBase64)
			keep := n
			if keep > 1048576 {
				keep = 1048576
			}
			if !bytes.Equal(a, bytes.Repeat([]byte{255}, keep)) || !bytes.Equal(b, bytes.Repeat([]byte{254}, keep)) || r.StdoutTruncated != (n > keep) || r.StderrTruncated != (n > keep) {
				t.Fatal(n, r.Status)
			}
		}
		r := run(t, "/bin/echo", []any{"private"}, false, 0)
		if r.StdoutBase64 != nil || r.StderrBase64 != nil {
			t.Fatal("output exposed")
		}
	})
	t.Run("product-exits", func(t *testing.T) {
		r := run(t, "/usr/bin/python3", []any{"-I", "-c", "import time;time.sleep(30)"}, false, 1, "--timeout", "200ms")
		if r.Changed != nil || r.ExitCode != nil || r.Status != result.Failed {
			t.Fatal("product timeout", r)
		}
		for _, code := range []int{1, 23, 255, -9, -15} {
			script := fmt.Sprintf("import sys;sys.exit(%d)", code)
			if code < 0 {
				script = fmt.Sprintf("import os;os.kill(os.getpid(),%d)", -code)
			}
			r := run(t, "/usr/bin/python3", []any{"-I", "-c", script}, false, 1)
			if r.ExitCode == nil || *r.ExitCode != code || r.Changed != nil {
				t.Fatal(code, r)
			}
		}
		for _, exe := range []string{"/does-not-exist", "/etc/shadow", "/etc/debian_version"} {
			r := run(t, exe, []any{}, false, 1)
			if r.ExitCode != nil || r.Changed == nil || *r.Changed {
				t.Fatal(exe, r)
			}
		}
		r = run(t, "/usr/bin/python3", []any{"-I", "-c", "import sys;assert sys.stdin.buffer.read()==b''"}, false, 0)
		if r.Changed == nil || !*r.Changed {
			t.Fatal(r)
		}
		// Executable text without a shebang must fail ENOEXEC, never fall back
		// to a shell. The explicit shebang variant is allowed.
		run(t, "/usr/bin/python3", []any{"-I", "-c", "import os; p='/fixture/results/noexec';open(p,'w').write('exit 0\\n');os.chmod(p,0o700);p='/fixture/results/shebang';open(p,'w').write('#!/bin/sh\\nexit 0\\n');os.chmod(p,0o700)"}, false, 0)
		r = run(t, "/fixture/results/noexec", []any{}, false, 1)
		if r.Changed == nil || *r.Changed || r.ExitCode != nil {
			t.Fatal("ENOEXEC", r)
		}
		run(t, "/fixture/results/shebang", []any{}, false, 0)
	})
	t.Run("receiver-interruption", func(t *testing.T) {
		for _, mode := range []string{"timeout", "cancel", "disconnect"} {
			t.Run(mode, func(t *testing.T) {
				s, e := (transport.SSHConnector{}).Connect(ctx, host)
				if e != nil {
					t.Fatal(e)
				}
				defer s.Close()
				c, stop := context.WithTimeout(ctx, 200*time.Millisecond)
				defer stop()
				if mode != "timeout" {
					c, stop = context.WithCancel(ctx)
					defer stop()
					go func() {
						time.Sleep(200 * time.Millisecond)
						if mode == "cancel" {
							stop()
						} else {
							s.Close()
						}
					}()
				}
				start := time.Now()
				response, e := s.Run(c, transport.Request{Command: LinuxCommandWrapper(), Stdin: request("/usr/bin/python3", []any{"-I", "-c", "import time;time.sleep(30)"}), StdoutLimit: transport.WrapperResponseLimit})
				if e == nil || time.Since(start) > 5*time.Second || DecodeLinuxCommand(response, false).Changed != nil {
					t.Fatal("interruption", e)
				}
			})
		}
	})
}
