package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/transport"
)

type delegateConnector struct {
	calls  int
	marker string
	t      *testing.T
}

func TestDelegateForksWindows(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, x any) string {
		data, err := json.Marshal(x)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err = os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	hosts := map[string]any{}
	for _, name := range []string{"a", "b", "c", "d"} {
		hosts[name] = map[string]any{"vars": map[string]string{"marker": filepath.Join(dir, name+".started")}}
	}
	inv := write("inv.json", map[string]any{"version": 1, "hosts": hosts})
	release := filepath.Join(dir, "release")
	script := `[IO.File]::WriteAllText('${marker}','started'); while (!(Test-Path -LiteralPath '` + strings.ReplaceAll(release, "'", "''") + `')) { Start-Sleep -Milliseconds 20 }`
	tasks := write("tasks.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "barrier", "module": "command", "delegate_to": "localhost", "args": map[string]any{"executable": filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "argv": []string{"-NoProfile", "-NonInteractive", "-Command", script}}}}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan int, 1)
	go func() {
		var out, diag bytes.Buffer
		done <- RunEngine(ctx, []string{"run", "--inventory", inv, "--tasks", tasks, "--forks", "3"}, &out, &diag, engine.Runtime{})
	}()
	count := func() int { files, _ := filepath.Glob(filepath.Join(dir, "*.started")); return len(files) }
	deadline := time.Now().Add(10 * time.Second)
	for count() < 3 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	if n := count(); n != 3 {
		cancel()
		<-done
		t.Fatalf("forks=3 started %d processes before release", n)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	if code := <-done; code != 0 || count() != 4 {
		t.Fatalf("exit=%d started=%d", code, count())
	}
}

func (c *delegateConnector) Connect(context.Context, transport.Host) (transport.Session, error) {
	c.calls++
	if _, err := os.Stat(c.marker); err != nil {
		c.t.Fatal("SSH connected before local command completed")
	}
	return nil, errors.New("SECRET connection error")
}

func TestDelegateCLIWindows(t *testing.T) {
	dir := t.TempDir()
	writeJSON := func(name string, x any) string {
		b, err := json.Marshal(x)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err = os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	inv := writeJSON("inventory.json", map[string]any{"version": 1, "hosts": map[string]any{
		"a": map[string]any{"os": "linux", "vars": map[string]string{"message": "first"}},
		"b": map[string]any{"os": "linux", "vars": map[string]string{"message": "second"}},
	}})
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	task := func(script string) map[string]any {
		return map[string]any{"name": "local", "module": "command", "delegate_to": "localhost", "become": false, "args": map[string]any{"executable": exe, "argv": []string{"-NoProfile", "-NonInteractive", "-Command", script}}}
	}
	marker := filepath.Join(dir, "marker")
	first := task(`[IO.File]::AppendAllText('` + strings.ReplaceAll(marker, "'", "''") + `', '${message}'+[Environment]::NewLine); [Console]::Write('${message}')`)
	tasks := writeJSON("tasks.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{first}})
	c := &delegateConnector{marker: marker, t: t}
	run := func(extra ...string) (int, string, string) {
		var out, diag bytes.Buffer
		args := []string{"run", "--inventory", inv, "--tasks", tasks, "--format", "json", "--forks", "1"}
		args = append(args, extra...)
		code := RunEngine(context.Background(), args, &out, &diag, engine.Runtime{Connector: c})
		return code, out.String(), diag.String()
	}
	code, out, diag := run("--include-output")
	if code != 0 || c.calls != 0 || diag != "" {
		t.Fatalf("%d %s %s calls=%d", code, out, diag, c.calls)
	}
	for _, value := range []string{"first", "second"} {
		if !strings.Contains(out, base64.StdEncoding.EncodeToString([]byte(value))) {
			t.Fatal(out)
		}
	}
	data, _ := os.ReadFile(marker)
	if string(data) != "first\r\nsecond\r\n" {
		t.Fatalf("execution count/order: %q", data)
	}
	if !strings.Contains(out, `"host":"a"`) && !strings.Contains(out, `"host": "a"`) {
		t.Fatal(out)
	}
	code, out, _ = run()
	if code != 0 || strings.Contains(out, "stdout_base64") {
		t.Fatal(out)
	}
	// Entire preflight must finish before the first marker-producing task runs.
	os.Remove(marker)
	invalid := task("exit 0")
	invalid["args"].(map[string]any)["executable"] = "relative.exe"
	tasks = writeJSON("bad.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{first, invalid}})
	code, _, _ = run()
	if code != 2 {
		t.Fatal(code)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("preflight executed a command")
	}
	// Local failure skips subsequent local tasks.
	tasks = writeJSON("fail.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{task("exit 7"), first}})
	code, out, _ = run()
	if code != 1 || !strings.Contains(out, "skipped") {
		t.Fatalf("%d %s", code, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("failed host continued")
	}
	// Local first, then SSH, with failure attributed to the original host/task.
	inv = writeJSON("remote.json", map[string]any{"version": 1, "hosts": map[string]any{"a": map[string]any{"os": "linux", "address": "unused", "user": "unused", "private_key_file": "missing", "known_hosts_file": "missing", "vars": map[string]string{"message": "first"}}}})
	remote := map[string]any{"name": "remote", "module": "command", "args": map[string]any{"executable": "/bin/true", "argv": []string{}}}
	tasks = writeJSON("mixed.json", map[string]any{"version": 1, "hosts": "all", "tasks": []any{first, remote, first}})
	code, out, diag = run()
	if code != 1 || c.calls != 1 || !strings.Contains(out, "unreachable") || !strings.Contains(out, "skipped") || strings.Contains(out+diag, "SECRET") {
		t.Fatalf("%d %s %s calls=%d", code, out, diag, c.calls)
	}
}
