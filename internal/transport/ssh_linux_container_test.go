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
	"runtime"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// TestLinuxOpenSSH is deliberately separate from receiver and Linux CLI tests.
// Its only inputs are synthetic, so JSON test evidence contains no task secrets.
func TestLinuxOpenSSH(t *testing.T) {
	dir := os.Getenv("WINTABLE_LINUX_SSH_FIXTURE")
	if dir == "" {
		t.Skip("run tests/linux-ssh/run.ps1")
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Fatal("fixture requires Windows amd64 controller")
	}
	var cfg struct {
		Address, User, Container, DockerContext string
		Port                                    uint16
	}
	raw, err := os.ReadFile(filepath.Join(dir, "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1" || cfg.Port == 0 || cfg.User != "runner" {
		t.Fatal("unsafe fixture endpoint")
	}
	host := Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(dir, "client"), KnownHostsFile: filepath.Join(dir, "known_hosts")}
	docker := func(args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "docker", append([]string{"--context", cfg.DockerContext, "exec", cfg.Container}, args...)...)
		data, e := cmd.Output()
		if e != nil {
			t.Fatalf("management observation failed: %v", e)
		}
		return data
	}
	starts := func() []byte { return docker("/bin/sh", "-c", "cat /fixture/results/starts 2>/dev/null || true") }
	connect := func(h Host) (Session, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		return (SSHConnector{}).Connect(ctx, h)
	}
	t.Run("reject", func(t *testing.T) {
		before := starts()
		trusted, e := os.ReadFile(host.KnownHostsFile)
		if e != nil {
			t.Fatal(e)
		}
		for _, mode := range []string{"unknown", "changed", "auth", "root", "password"} {
			t.Run(mode, func(t *testing.T) {
				h := host
				want := "authentication_failed"
				switch mode {
				case "unknown":
					h.KnownHostsFile = filepath.Join(t.TempDir(), "known")
					e = os.WriteFile(h.KnownHostsFile, nil, 0600)
					want = "host_key_unknown"
				case "changed":
					signer, _ := testKey(t)
					h.KnownHostsFile = filepath.Join(t.TempDir(), "known")
					addr := net.JoinHostPort(h.Address, strconv.Itoa(int(h.Port)))
					e = os.WriteFile(h.KnownHostsFile, []byte(knownhosts.Line([]string{knownhosts.Normalize(addr)}, signer.PublicKey())+"\n"), 0600)
					want = "host_key_changed"
				case "auth":
					_, key := testKey(t)
					h.PrivateKeyFile = filepath.Join(dir, "wrong-client")
					defer os.Remove(h.PrivateKeyFile)
					e = os.WriteFile(h.PrivateKeyFile, key, 0600)
				case "root":
					h.User = "root"
				}
				if e != nil {
					t.Fatal(e)
				}
				if mode == "password" {
					verify, err := knownhosts.New(host.KnownHostsFile)
					if err != nil {
						t.Fatal(err)
					}
					addr := net.JoinHostPort(host.Address, strconv.Itoa(int(host.Port)))
					conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(5 * time.Second))
					client, _, _, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{User: "runner", Auth: []ssh.AuthMethod{ssh.Password("synthetic-fixture-password")}, HostKeyCallback: verify})
					if client != nil {
						client.Close()
						t.Fatal("password accepted")
					}
					if err == nil {
						t.Fatal("password accepted")
					}
					t.Log("expected/actual password authentication rejected; exec_count=0")
					return
				}
				s, e := connect(h)
				if s != nil {
					s.Close()
					t.Fatal("rejection accepted")
				}
				if e == nil || e.Error() != want {
					t.Fatalf("expected %s, observed %v", want, e)
				}
				t.Logf("expected=%s actual=%s exec_count=0", want, e)
			})
		}
		after, e := os.ReadFile(host.KnownHostsFile)
		if e != nil || !bytes.Equal(trusted, after) || !bytes.Equal(before, starts()) {
			t.Fatal("rejection modified trust or launched probe")
		}
	})
	// Exactly 2MiB valid UTF-8 with multi-byte codepoints crossing 7-byte reads.
	payload := bytes.Repeat([]byte("あa"), 1<<19)
	sum := sha256.Sum256(payload)
	type observation struct {
		Length     int
		SHA256     string
		EOF        bool
		SplitReads int `json:"split_reads"`
		UID        int
	}
	checkRecord := func(t *testing.T, raw []byte) {
		t.Helper()
		var o observation
		if e := json.Unmarshal(raw, &o); e != nil {
			t.Fatal(e)
		}
		if o.Length != len(payload) || o.SHA256 != hex.EncodeToString(sum[:]) || !o.EOF || o.SplitReads == 0 || o.UID == 0 {
			t.Fatalf("hash/EOF/uid observation mismatch: %+v", o)
		}
		t.Logf("expected/actual length=%d sha256=%s eof=true uid=%d split_reads=%d", o.Length, o.SHA256, o.UID, o.SplitReads)
	}
	t.Run("reuse_hash_flood", func(t *testing.T) {
		before := bytes.Count(starts(), []byte("\n"))
		s, e := connect(host)
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		for _, mode := range []string{"hash", "flood", "hash"} {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			r, e := s.Run(ctx, Request{Command: "/usr/bin/python3 -I /fixture/probe.py " + mode, Stdin: payload})
			cancel()
			if e != nil || r.ExitCode == nil || r.Communication != CommunicationComplete {
				t.Fatalf("%s failed: %v", mode, e)
			}
			if mode == "hash" {
				if *r.ExitCode != 0 || len(r.Stderr) != 0 || r.StdoutTruncated || r.StderrTruncated {
					t.Fatal("hash status mismatch")
				}
				checkRecord(t, r.Stdout)
			} else {
				if *r.ExitCode != 23 || !r.StdoutTruncated || !r.StderrTruncated || !bytes.Equal(r.Stdout, bytes.Repeat([]byte("O"), CaptureLimit)) || !bytes.Equal(r.Stderr, bytes.Repeat([]byte("E"), CaptureLimit)) {
					t.Fatal("flood capture mismatch")
				}
				checkRecord(t, docker("cat", "/fixture/results/flood.json"))
				t.Log("expected/actual exit=23 stdout=1048576 O stderr=1048576 E both_truncated=true")
			}
		}
		if bytes.Count(starts(), []byte("\n"))-before != 3 {
			t.Fatal("unexpected retry or missing exec")
		}
	})
	for _, mode := range []string{"no-read", "wait"} {
		t.Run("deadline_"+mode, func(t *testing.T) {
			before := bytes.Count(starts(), []byte("\n"))
			s, e := connect(host)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()
			begin := time.Now()
			input := payload
			if mode == "no-read" {
				input = bytes.Repeat([]byte("a"), 16<<20)
			}
			_, e = s.Run(ctx, Request{Command: "/usr/bin/python3 -I /fixture/probe.py " + mode, Stdin: input})
			if e == nil || e.Error() != "deadline_exceeded" || time.Since(begin) > 5*time.Second {
				t.Fatalf("deadline release failed: %v", e)
			}
			if bytes.Count(starts(), []byte("\n"))-before != 1 {
				t.Fatal("unexpected retry or missing exec")
			}
			t.Log("expected/actual deadline_exceeded; exec_count=1; released within 5s")
		})
	}
	for _, mode := range []string{"cancel", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			docker("rm", "-f", "/fixture/results/wait.json")
			before := bytes.Count(starts(), []byte("\n"))
			s, e := connect(host)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, e := s.Run(ctx, Request{Command: "/usr/bin/python3 -I /fixture/probe.py wait", Stdin: payload})
				done <- e
			}()
			// Observe EOF before cancelling/closing; don't infer remote process kill.
			end := time.Now().Add(8 * time.Second)
			for len(docker("/bin/sh", "-c", "cat /fixture/results/wait.json 2>/dev/null || true")) == 0 {
				if time.Now().After(end) {
					t.Fatal("EOF observation timeout")
				}
				time.Sleep(50 * time.Millisecond)
			}
			checkRecord(t, docker("cat", "/fixture/results/wait.json"))
			want := "connection_lost"
			if mode == "cancel" {
				want = "cancelled"
				cancel()
			} else {
				s.Close()
			}
			select {
			case e := <-done:
				if e == nil || e.Error() != want {
					t.Fatalf("expected %s actual %v", want, e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("session did not release")
			}
			if bytes.Count(starts(), []byte("\n"))-before != 1 {
				t.Fatal("unexpected retry or missing exec")
			}
			t.Logf("expected/actual %s; exec_count=1; released within 5s", want)
		})
	}
}
