package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

// Local tests use actual Windows PowerShell 5.1 and cmd, never pwsh or Go argv.
// The opt-in SSH variant is provisioned only by the disposable container fixture.
func TestCommandWindows(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "日本語 空白.exe")
	source, err := filepath.Abs("../../tests/command/CRTReceiver.cs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	builder, err := filepath.Abs("../../tests/command/build.ps1")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-File", builder, "-Source", source, "-Output", exe).CombinedOutput(); err != nil {
		t.Fatalf("CRT fixture compile: %v: %s", err, out)
	}
	t.Logf("CRT parser: msvcrt.dll __wgetmainargs; local exe=%s", exe)
	if out, err := exec.CommandContext(ctx, exe, "direct").CombinedOutput(); err != nil {
		t.Fatalf("direct CRT fixture: %v %s", err, out)
	}
	modes := []string{"cmd"}
	if os.Getenv("WINTABLE_SSH_FIXTURE") != "" {
		modes = append(modes, "ssh")
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			target := exe
			var session transport.Session
			if mode == "ssh" {
				fixture := os.Getenv("WINTABLE_SSH_FIXTURE")
				raw, e := os.ReadFile(filepath.Join(fixture, "fixture.json"))
				if e != nil {
					t.Fatal(e)
				}
				var cfg struct {
					Address, User string
					Port          uint16
				}
				if json.Unmarshal(raw, &cfg) != nil {
					t.Fatal("fixture JSON")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				session, e = (transport.SSHConnector{}).Connect(ctx, transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(fixture, "client"), KnownHostsFile: filepath.Join(fixture, "known_hosts")})
				if e != nil {
					t.Fatal(e)
				}
				defer session.Close()
				target = `C:\fixture\日本語 空白.exe`
			}
			run := func(input []byte) transport.Response {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if session != nil {
					r, e := session.Run(ctx, transport.Request{Command: CommandWrapper(), Stdin: input, StdoutLimit: transport.WrapperResponseLimit})
					if e != nil {
						t.Fatal(e)
					}
					return r
				}
				c := exec.CommandContext(ctx, "cmd.exe")
				c.SysProcAttr = rawCommand(CommandWrapper())
				c.Stdin = bytes.NewReader(input)
				if len(input) < 4096 {
					c.Stdin = &sevenBytes{bytes.NewReader(input)}
				}
				var o, e bytes.Buffer
				c.Stdout = &o
				c.Stderr = &e
				err := c.Run()
				code := 0
				if err != nil {
					if x, ok := err.(*exec.ExitError); ok {
						code = x.ExitCode()
					} else {
						t.Fatal(err)
					}
				}
				return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code, Stdout: o.Bytes(), Stderr: e.Bytes()}
			}
			payload := func(args []string) []byte {
				values := make([]any, len(args))
				for i, s := range args {
					values[i] = s
				}
				p, e := Prepare("h", 1, "command", map[string]any{"executable": target, "argv": values})
				if e != nil {
					t.Fatal(e)
				}
				return p.JSON()
			}
			t.Run("CRT_roundtrip", func(t *testing.T) {
				args := []string{"", "a b", "日本語", `a"b`, `a\\"b`, `trailing space\`, `\\`, "😀", "& | < > ^ %PATH% !x! $();`\r\n", strings.Repeat("x", 9000)}
				r := run(payload(args))
				row := DecodeCommand(r, true)
				if row.Status != result.Changed || row.ExitCode == nil || *row.ExitCode != 0 {
					t.Fatalf("%+v outer=%+v", row, r)
				}
				out, e := base64.StdEncoding.DecodeString(*row.StdoutBase64)
				if e != nil {
					t.Fatal(e)
				}
				lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
				got := []string{}
				for _, line := range lines {
					b, e := base64.StdEncoding.DecodeString(line)
					if e != nil {
						t.Fatal(e)
					}
					units := make([]uint16, len(b)/2)
					for i := range units {
						units[i] = binary.LittleEndian.Uint16(b[i*2:])
					}
					got = append(got, string(utf16.Decode(units)))
				}
				if !reflect.DeepEqual(args, got) {
					t.Fatalf("CRT argv mismatch: %q", got)
				}
			})
			t.Run("nonzero_and_bytes", func(t *testing.T) {
				row := DecodeCommand(run(payload([]string{"--exit", "13"})), false)
				if row.Status != result.Failed || row.Changed != nil || row.ExitCode == nil || *row.ExitCode != 13 {
					t.Fatal(row)
				}
				r := run(payload([]string{"--flood"}))
				row = DecodeCommand(r, true)
				if row.ExitCode == nil || *row.ExitCode != 7 || !row.StdoutTruncated || !row.StderrTruncated {
					t.Fatalf("%+v response bytes=%d", row, len(r.Stdout))
				}
				for _, body := range []*string{row.StdoutBase64, row.StderrBase64} {
					b, e := base64.StdEncoding.DecodeString(*body)
					if e != nil || len(b) != transport.CaptureLimit {
						t.Fatal("capture size", e)
					}
					for i, v := range b {
						if v != byte(i%256) {
							t.Fatal("bytes changed")
						}
					}
				}
				hidden := DecodeCommand(r, false)
				if hidden.StdoutBase64 != nil || hidden.StderrBase64 != nil || hidden.StdoutTruncated {
					t.Fatal("default exposes output")
				}
			})
			t.Run("receiver_boundaries", func(t *testing.T) {
				for _, n := range []int{32765, 32766, 32767} {
					arg := strings.Repeat("x", n-UTF16Len(`"`+target+`"`)-1)
					input, _ := json.Marshal(map[string]any{"module": "command", "args": map[string]any{"executable": target, "argv": []string{arg}}})
					row := DecodeCommand(run(input), false)
					if (row.Status == result.Changed) != (n <= 32766) {
						t.Fatalf("line %d: %+v", n, row)
					}
				}
				seed := payload([]string{})
				for _, n := range []int{JSONLimit - 1, JSONLimit, JSONLimit + 1} {
					input := append(append([]byte{}, seed...), bytes.Repeat([]byte(" "), n-len(seed))...)
					row := DecodeCommand(run(input), false)
					if (row.Status == result.Changed) != (n <= JSONLimit) {
						t.Fatalf("JSON %d: %+v", n, row)
					}
				}
			})
			t.Run("invalid_before_execution", func(t *testing.T) {
				marker := filepath.Join(dir, "must-not-exist")
				if mode == "ssh" {
					marker = `C:\fixture\must-not-exist`
				}
				valid := payload([]string{"--marker", marker})
				invalid := [][]byte{nil, []byte("[]"), []byte("null"), []byte(`{"module":"command","args":null}`), []byte(`{"module":"command","module":"command"}`), append(append([]byte{}, valid...), []byte(" {}")...), append([]byte{0xef, 0xbb, 0xbf}, valid...), append([]byte{0xff}, valid...), bytes.Replace(valid, []byte(`"module"`), []byte(`"unknown"`), 1), bytes.Replace(valid, []byte(`"module"`), []byte(`"args":{},"module"`), 1), bytes.Replace(valid, []byte(`"command"`), []byte(`"\u+001"`), 1), bytes.Replace(valid, []byte(`"command"`), []byte(`"\ud800"`), 1), bytes.Replace(valid, []byte(`"argv"`), []byte(`"argv":[],"argv"`), 1), []byte(`{"module":"command","args":{"executable":"C:\\Windows\\System32\\cmd.exe","argv":[]}}`)}
				for i, input := range invalid {
					r := run(input)
					if string(r.Stdout) != `{"error":"invalid_request"}` || len(r.Stderr) != 0 {
						t.Fatalf("case %d: outer=%+v", i, r)
					}
				}
				if session == nil {
					if _, e := os.Stat(marker); !os.IsNotExist(e) {
						t.Fatal("invalid input executed child")
					}
				} else {
					row := DecodeCommand(run(payload([]string{"--assert-absent", marker})), false)
					if row.ExitCode == nil || *row.ExitCode != 0 {
						t.Fatal("invalid input executed remote child")
					}
				}
			})
			if session == nil {
				t.Run("EOF_required", func(t *testing.T) {
					marker := filepath.Join(dir, "eof-marker")
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					c := exec.CommandContext(ctx, "cmd.exe")
					c.SysProcAttr = rawCommand(CommandWrapper())
					stdin, e := c.StdinPipe()
					if e != nil {
						t.Fatal(e)
					}
					var out, diag bytes.Buffer
					c.Stdout = &out
					c.Stderr = &diag
					if e = c.Start(); e != nil {
						t.Fatal(e)
					}
					if _, e = stdin.Write(payload([]string{"--marker", marker})); e != nil {
						t.Fatal(e)
					}
					done := make(chan error, 1)
					go func() { done <- c.Wait() }()
					select {
					case e := <-done:
						t.Fatalf("exited before EOF: %v", e)
					case <-time.After(2 * time.Second):
					}
					if _, e = os.Stat(marker); !os.IsNotExist(e) {
						t.Fatal("executed before EOF")
					}
					if e = stdin.Close(); e != nil {
						t.Fatal(e)
					}
					if e = <-done; e != nil {
						t.Fatal(e)
					}
					if _, e = os.Stat(marker); e != nil {
						t.Fatal("did not execute after EOF", e)
					}
				})
			}
			t.Logf("launch units=%d mode=%s", UTF16Len(WrapperLaunchLine(CommandWrapper())), mode)
		})
	}
}

type sevenBytes struct{ io.Reader }

func (s *sevenBytes) Read(p []byte) (int, error) {
	if len(p) > 7 {
		p = p[:7]
	}
	return s.Reader.Read(p)
}
