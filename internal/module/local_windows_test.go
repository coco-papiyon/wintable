package module

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/coco-papiyon/wintable/internal/result"
	"golang.org/x/sys/windows"
)

func utf16Bytes(s string) []byte {
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[i*2:], u)
	}
	return b
}

func TestLocalCommandWindows(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "日本語 空白.exe")
	source, _ := filepath.Abs("../../tests/command/CRTReceiver.cs")
	builder, _ := filepath.Abs("../../tests/command/build.ps1")
	if out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", builder, "-Source", source, "-Output", exe).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v %s", err, out)
	}
	run := func(ctx context.Context, args []string, output bool) result.TaskResult {
		t.Helper()
		values := make([]any, len(args))
		for i, a := range args {
			values[i] = a
		}
		p, err := Prepare("original", 1, "command", map[string]any{"executable": exe, "argv": values})
		if err != nil {
			t.Fatal(err)
		}
		return RunLocalCommand(ctx, p, output)
	}
	t.Run("CRT", func(t *testing.T) {
		args := []string{"", "a b", "日本語", `a"b`, `trailing space\`, "😀", "& | < > ^ %PATH% !x! $();`\r\n"}
		row := run(context.Background(), args, true)
		var want strings.Builder
		for _, a := range args {
			for _, u := range []string{a} {
				want.WriteString(base64.StdEncoding.EncodeToString(utf16Bytes(u)) + "\n")
			}
		}
		if row.Status != result.Changed || row.StdoutBase64 == nil {
			t.Fatal(row)
		}
		out, _ := base64.StdEncoding.DecodeString(*row.StdoutBase64)
		if string(out) != want.String() {
			t.Fatalf("CRT mismatch: %q", out)
		}
	})
	t.Run("flood", func(t *testing.T) {
		row := run(context.Background(), []string{"--flood"}, true)
		if row.Status != result.Failed || row.Error.Code != "task_failed" || row.ExitCode == nil || *row.ExitCode != 7 || !row.StdoutTruncated || !row.StderrTruncated {
			t.Fatal(row)
		}
		for _, p := range []*string{row.StdoutBase64, row.StderrBase64} {
			b, _ := base64.StdEncoding.DecodeString(*p)
			if len(b) != 1<<20 {
				t.Fatal(len(b))
			}
		}
	})
	t.Run("stdin-EOF", func(t *testing.T) {
		if row := run(context.Background(), []string{"--stdin-eof"}, false); row.Status != result.Changed {
			t.Fatal(row)
		}
	})
	t.Run("output-boundaries", func(t *testing.T) {
		for _, n := range []int{(1 << 20) - 1, 1 << 20, (1 << 20) + 1} {
			row := run(context.Background(), []string{"--bytes", strconv.Itoa(n)}, true)
			if row.Status != result.Changed || row.StdoutTruncated != (n > 1<<20) || row.StderrTruncated != (n > 1<<20) {
				t.Fatal(row)
			}
			for _, p := range []*string{row.StdoutBase64, row.StderrBase64} {
				data, _ := base64.StdEncoding.DecodeString(*p)
				want := n
				if want > 1<<20 {
					want = 1 << 20
				}
				if len(data) != want {
					t.Fatal(len(data))
				}
			}
		}
	})
	t.Run("command-line-boundaries", func(t *testing.T) {
		for _, n := range []int{32765, 32766, 32767} {
			arg := strings.Repeat("x", n-UTF16Len(CommandLine(exe, []string{""}))+2)
			p, err := Prepare("h", 1, "command", map[string]any{"executable": exe, "argv": []any{arg}})
			if n > CommandLimit {
				if err == nil {
					t.Fatal("accepted over limit")
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			row := RunLocalCommand(context.Background(), p, true)
			if row.Status != result.Changed {
				t.Fatalf("%d: %+v", n, row)
			}
		}
	})
	for _, mode := range []string{"--tree", "--orphan"} {
		t.Run(mode, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "child")
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			done := make(chan result.TaskResult, 1)
			go func() { done <- run(ctx, []string{mode, marker}, false) }()
			deadline := time.Now().Add(10 * time.Second)
			var data []byte
			for time.Now().Before(deadline) {
				data, _ = os.ReadFile(marker)
				if len(data) > 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			pid, err := strconv.Atoi(string(data))
			if err != nil {
				stop()
				<-done
				t.Fatal("child did not start")
			}
			var handle windows.Handle
			if mode == "--orphan" {
				// Keep the exact child process object alive while RunLocalCommand
				// tears down the job; reopening by PID afterward could observe a
				// different process if Windows reuses the identifier.
				handle, err = windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
				if err != nil {
					stop()
					<-done
					t.Fatalf("open child process: %v", err)
				}
				defer windows.CloseHandle(handle)
				var membership []byte
				for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
					membership, _ = os.ReadFile(marker + ".started.job")
					if len(membership) > 0 {
						break
					}
					time.Sleep(time.Millisecond)
				}
				if string(membership) != "true" {
					stop()
					<-done
					t.Fatalf("descendant job membership: %q", membership)
				}
				if err := os.WriteFile(marker+".release", []byte("release"), 0600); err != nil {
					stop()
					<-done
					t.Fatal(err)
				}
			}
			if mode == "--tree" {
				stop()
			}
			row := <-done
			if mode == "--orphan" && row.Status != result.Changed {
				t.Fatal(row)
			}
			if mode == "--tree" && (row.Error == nil || row.Error.Code != "deadline_exceeded") {
				t.Fatal(row)
			}
			if mode == "--tree" {
				handle, err = windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
				if err == nil {
					defer windows.CloseHandle(handle)
				}
			}
			if err == nil {
				state, e := windows.WaitForSingleObject(handle, 0)
				if e != nil || state != windows.WAIT_OBJECT_0 {
					t.Fatalf("descendant still alive: %v %d", e, state)
				}
			}
		})
	}
	t.Run("private", func(t *testing.T) {
		row := run(context.Background(), []string{"SECRET"}, false)
		if row.Status != result.Changed || row.StdoutBase64 != nil || row.StderrBase64 != nil {
			t.Fatal(row)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		ctx, stop := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer stop()
		row := run(ctx, []string{"--wait", filepath.Join(t.TempDir(), "marker")}, false)
		if row.Status != result.Failed || row.Error.Code != "deadline_exceeded" {
			t.Fatal(row)
		}
	})
	t.Run("missing", func(t *testing.T) {
		p, _ := Prepare("h", 1, "command", map[string]any{"executable": filepath.Join(t.TempDir(), "missing.exe"), "argv": []any{}})
		row := RunLocalCommand(context.Background(), p, false)
		if row.Status != result.Failed || row.Error.Code != "task_failed" || row.Changed == nil || *row.Changed {
			t.Fatal(row)
		}
	})
}
