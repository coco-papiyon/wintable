package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/result"
)

// The helper owns a fresh console so CTRL_C_EVENT never reaches the caller.
func TestWindowsCLIConsoleHelper(t *testing.T) {
	if os.Getenv("WINTABLE_CONSOLE_HELPER") != "1" {
		return
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	if ok, _, err := kernel.NewProc("AllocConsole").Call(); ok == 0 {
		t.Fatal(err)
	}
	defer kernel.NewProc("FreeConsole").Call()
	handler := syscall.NewCallback(func(event uint32) uintptr { return 1 })
	if ok, _, err := kernel.NewProc("SetConsoleCtrlHandler").Call(handler, 1); ok == 0 {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Getenv("WINTABLE_VALIDATION_EXE"), "run", "--inventory", os.Getenv("WINTABLE_PROCESS_INVENTORY"), "--tasks", os.Getenv("WINTABLE_PROCESS_TASKS"), "--format", "json")
	var output, diagnostic bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &diagnostic
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	// Wait for the target executable's marker, rather than guessing SSH startup time.
	ready := false
	for start := time.Now(); time.Since(start) < 20*time.Second; {
		probe := exec.CommandContext(ctx, "docker", "exec", os.Getenv("WINTABLE_PROCESS_CONTAINER"), "cmd.exe", "/c", "type", `C:\fixture\cli-cancel.txt`)
		data, err := probe.Output()
		if err == nil && string(data) == "started\r\n" || err == nil && string(data) == "started\n" {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("target command did not start exactly once")
	}
	if ok, _, err := kernel.NewProc("GenerateConsoleCtrlEvent").Call(0, 0); ok == 0 {
		t.Fatal(err)
	}
	err := cmd.Wait()
	if cmd.ProcessState.ExitCode() != 130 {
		t.Fatalf("Ctrl+C exit=%d err=%v stderr=%s", cmd.ProcessState.ExitCode(), err, diagnostic.String())
	}
	var report result.Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"changed":null`)) {
		t.Fatal("cancelled change state must be null")
	}
	t.Logf("Actual CTRL_C_EVENT: exit=130; single JSON; changed=null; output=%s", output.String())
}

func TestWindowsCLIProcess(t *testing.T) {
	fixture := os.Getenv("WINTABLE_SSH_FIXTURE")
	executable := os.Getenv("WINTABLE_VALIDATION_EXE")
	if fixture == "" || executable == "" {
		t.Skip("run validation runner with disposable SSH fixture")
	}
	raw, err := os.ReadFile(filepath.Join(fixture, "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Address, User, Container string
		Port                     uint16
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.json")
	tasks := filepath.Join(dir, "tasks.json")
	write := func(path string, value any) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(inventory, map[string]any{"version": 1, "hosts": map[string]any{"target": map[string]any{"address": cfg.Address, "port": cfg.Port, "user": cfg.User, "private_key_file": filepath.Join(fixture, "client"), "known_hosts_file": filepath.Join(fixture, "known_hosts")}}})
	setTask := func(argv []string) {
		write(tasks, map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "process", "module": "command", "args": map[string]any{"executable": `C:\fixture\日本語 空白.exe`, "argv": argv}}}})
	}
	t.Run("V10_input_encodings_V12_formats", func(t *testing.T) {
		setTask([]string{"日本語", "space value", ""})
		originalInventory, _ := os.ReadFile(inventory)
		originalTasks, _ := os.ReadFile(tasks)
		for _, style := range []string{"LF", "CRLF", "BOM"} {
			for _, pair := range []struct {
				path string
				data []byte
			}{{inventory, originalInventory}, {tasks, originalTasks}} {
				var pretty bytes.Buffer
				if err := json.Indent(&pretty, pair.data, "", "  "); err != nil {
					t.Fatal(err)
				}
				data := pretty.Bytes()
				if style == "CRLF" {
					data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
				}
				if style == "BOM" {
					data = append([]byte{239, 187, 191}, data...)
				}
				if err := os.WriteFile(pair.path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, format := range []string{"json", "text"} {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				cmd := exec.CommandContext(ctx, executable, "run", "--inventory", inventory, "--tasks", tasks, "--format", format)
				data, err := cmd.CombinedOutput()
				cancel()
				if err != nil {
					t.Fatalf("%s/%s: %v %s", style, format, err, data)
				}
				if format == "json" {
					var report result.Report
					if err := json.Unmarshal(data, &report); err != nil {
						t.Fatal(err)
					}
					if bytes.Contains(data, []byte("stdout_base64")) || bytes.Contains(data, []byte("stderr_base64")) {
						t.Fatal("default output exposes body")
					}
				}
				t.Logf("%s/%s: actual exe over SSH exit=0; output=%s", style, format, data)
			}
		}
	})
	t.Run("V12_broken_stdout", func(t *testing.T) {
		setTask([]string{"--marker", `C:\fixture\cli-output.txt`})
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		reader.Close()
		defer writer.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "run", "--inventory", inventory, "--tasks", tasks, "--format", "json")
		cmd.Stdout = writer
		var diagnostic bytes.Buffer
		cmd.Stderr = &diagnostic
		err = cmd.Run()
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 3 {
			t.Fatalf("broken stdout: %v %s", err, diagnostic.String())
		}
		data, err := exec.CommandContext(ctx, "docker", "exec", cfg.Container, "cmd.exe", "/c", "type", `C:\fixture\cli-output.txt`).Output()
		if err != nil || string(data) != "executed" {
			t.Fatal("CLI did not execute target before output failure")
		}
		t.Log("Real product exe, real SSH command completed; closed stdout pipe => exit 3")
	})
	t.Run("V06_V12_CtrlC", func(t *testing.T) {
		setTask([]string{"--wait", `C:\fixture\cli-cancel.txt`})
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, self, "-test.run=^TestWindowsCLIConsoleHelper$", "-test.v")
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		cmd.Env = append(os.Environ(), "WINTABLE_CONSOLE_HELPER=1", "WINTABLE_PROCESS_INVENTORY="+inventory, "WINTABLE_PROCESS_TASKS="+tasks, "WINTABLE_PROCESS_CONTAINER="+cfg.Container)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("console helper: %v %s", err, output)
		}
		if !strings.Contains(string(output), "Actual CTRL_C_EVENT") {
			t.Fatal("missing cancellation assertion")
		}
		t.Log(string(output))
	})
}
