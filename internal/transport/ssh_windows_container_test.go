package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh/knownhosts"
)

// Opt-in: run through tests/windows-ssh/run.ps1, never a user's SSH service.
func TestWindowsOpenSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_SSH_FIXTURE")
	if dir == "" {
		t.Skip("run tests/windows-ssh/run.ps1")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "fixture.json"))
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
	host := Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")}
	docker := func(t *testing.T, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture docker command failed: %v: %s", err, data)
		}
		return data
	}
	remote := func(t *testing.T, path string) []byte {
		return docker(t, "exec", cfg.Container, "cmd.exe", "/c", "type", path)
	}
	connect := func(t *testing.T, h Host) Session {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s, e := (SSHConnector{}).Connect(ctx, h)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	command := func(mode string) string {
		return `powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File C:\fixture\receiver.ps1 -Mode ` + mode
	}
	t.Run("V05_reject_keys_and_auth", func(t *testing.T) {
		before, err := os.ReadFile(host.KnownHostsFile)
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"unknown", "changed", "auth"} {
			h := host
			want := "host_key_unknown"
			switch mode {
			case "unknown":
				h.KnownHostsFile = filepath.Join(t.TempDir(), "known")
				err = os.WriteFile(h.KnownHostsFile, nil, 0600)
			case "changed":
				signer, _ := testKey(t)
				h.KnownHostsFile = filepath.Join(t.TempDir(), "known")
				addr := net.JoinHostPort(h.Address, strconv.Itoa(int(h.Port)))
				err = os.WriteFile(h.KnownHostsFile, []byte(knownhosts.Line([]string{knownhosts.Normalize(addr)}, signer.PublicKey())+"\n"), 0600)
				want = "host_key_changed"
			case "auth":
				_, key := testKey(t)
				h.PrivateKeyFile = filepath.Join(t.TempDir(), "key")
				err = os.WriteFile(h.PrivateKeyFile, key, 0600)
				want = "authentication_failed"
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			s, e := (SSHConnector{}).Connect(ctx, h)
			cancel()
			if s != nil {
				s.Close()
				t.Fatal("invalid credentials accepted")
			}
			if e == nil || e.Error() != want {
				t.Fatalf("%s: %v", mode, e)
			}
		}
		after, err := os.ReadFile(host.KnownHostsFile)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("trust modified")
		}
		docker(t, "exec", cfg.Container, "powershell.exe", "-NoProfile", "-Command", `if (Test-Path C:\fixture\execs.log) { throw 'unexpected execution' }`)
	})
	t.Run("V11_V18_transfer_EOF_drain_reuse", func(t *testing.T) {
		s := connect(t, host)
		defer s.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		r, e := s.Run(ctx, Request{Command: command("version")})
		cancel()
		if e != nil || r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatalf("version: %v", e)
		}
		t.Logf("target environment: %s", bytes.TrimSpace(r.Stdout))
		// Valid BOM-free UTF-8 JSON, under 2 MiB; seven-byte remote reads split Japanese.
		payload := append([]byte(`{"message":"`), bytes.Repeat([]byte("日本語"), 230000)...)
		payload = append(payload, []byte(`"}`)...)
		sum := sha256.Sum256(payload)
		for range 2 {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			r, e := s.Run(ctx, Request{Command: command("transfer"), Stdin: payload})
			cancel()
			if e != nil || r.Communication != CommunicationComplete || r.ExitCode == nil || *r.ExitCode != 7 {
				t.Fatalf("transfer: %v", e)
			}
			if len(r.Stdout) != CaptureLimit || len(r.Stderr) != CaptureLimit || !r.StdoutTruncated || !r.StderrTruncated || !bytes.Equal(r.Stdout, bytes.Repeat([]byte("a"), CaptureLimit)) || !bytes.Equal(r.Stderr, bytes.Repeat([]byte("b"), CaptureLimit)) {
				t.Fatal("output capture/drain mismatch")
			}
			var got struct {
				Bytes  int
				SHA256 string
				EOF    bool
			}
			if err := json.Unmarshal(remote(t, `C:\fixture\received-transfer.json`), &got); err != nil {
				t.Fatal(err)
			}
			if got.Bytes != len(payload) || got.SHA256 != hex.EncodeToString(sum[:]) || !got.EOF {
				t.Fatal("stdin/EOF mismatch")
			}
		}
	})
	for _, mode := range []string{"blocked", "wait", "cancel", "close", "disconnect"} {
		t.Run("V06_V18_"+mode, func(t *testing.T) {
			s := connect(t, host)
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			remoteMode := mode
			if mode == "cancel" || mode == "close" {
				remoteMode = "blocked"
			}
			payload := bytes.Repeat([]byte("x"), 16<<20) // deliberately exceeds SSH window; transport stress only
			if mode == "wait" {
				payload = nil
			}
			done := make(chan struct{})
			if mode == "cancel" || mode == "close" {
				go func() {
					defer close(done)
					time.Sleep(2 * time.Second)
					if mode == "cancel" {
						cancel()
					} else {
						s.Close()
						s.Close()
					}
				}()
			} else {
				close(done)
			}
			start := time.Now()
			r, e := s.Run(ctx, Request{Command: command(remoteMode), Stdin: payload})
			<-done
			if e == nil || r.Communication == CommunicationComplete || r.ExitCode != nil || time.Since(start) > 12*time.Second {
				t.Fatalf("interruption: %v", e)
			}
			want := "connection_lost"
			if mode == "blocked" || mode == "wait" {
				want = "deadline_exceeded"
			}
			if mode == "cancel" {
				want = "cancelled"
			}
			if e.Error() != want {
				t.Fatalf("wanted %s, got %v", want, e)
			}
			retryCtx, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			if _, err := s.Run(retryCtx, Request{Command: command("version")}); err == nil {
				t.Fatal("interrupted connection reused")
			}
		})
	}
	// Fixed expected receiver count also catches implicit reconnect/replay and
	// rejection paths accidentally reaching exec. The disconnect case stops sshd.
	lines := strings.Fields(string(remote(t, `C:\fixture\execs.log`)))
	want := []string{"version", "transfer", "transfer", "blocked", "wait", "blocked", "blocked", "disconnect"}
	if strings.Join(lines, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected execs: %v", lines)
	}
}
