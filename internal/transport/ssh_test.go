package transport

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func testKey(t *testing.T) (ssh.Signer, []byte) {
	t.Helper()
	_, k, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	block, e := ssh.MarshalPrivateKey(k, "")
	if e != nil {
		t.Fatal(e)
	}
	raw := pem.EncodeToMemory(block)
	s, e := ssh.ParsePrivateKey(raw)
	if e != nil {
		t.Fatal(e)
	}
	return s, raw
}

type testPeer struct {
	host  Host
	execs atomic.Int32
	ptys  atomic.Int32
	wg    sync.WaitGroup
}

func startPeer(t *testing.T, handle func(ssh.Channel)) *testPeer {
	t.Helper()
	server, _ := testKey(t)
	user, key := testKey(t)
	config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(k.Marshal(), user.PublicKey().Marshal()) {
			return nil, nil
		}
		return nil, os.ErrPermission
	}}
	config.AddHostKey(server)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := &testPeer{}
	dir := t.TempDir()
	p.host = Host{Address: "127.0.0.1", Port: uint16(listener.Addr().(*net.TCPAddr).Port), User: "test", PrivateKeyFile: filepath.Join(dir, "key"), KnownHostsFile: filepath.Join(dir, "known")}
	if e = os.WriteFile(p.host.PrivateKeyFile, key, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p.host.KnownHostsFile, []byte(knownhosts.Line([]string{knownhosts.Normalize(listener.Addr().String())}, server.PublicKey())+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	var mu sync.Mutex
	var connections []net.Conn
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			raw, e := listener.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			connections = append(connections, raw)
			mu.Unlock()
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				defer raw.Close()
				c, channels, requests, e := ssh.NewServerConn(raw, config)
				if e != nil {
					return
				}
				defer c.Close()
				go ssh.DiscardRequests(requests)
				for ch := range channels {
					if ch.ChannelType() != "session" {
						ch.Reject(ssh.UnknownChannelType, "")
						continue
					}
					channel, reqs, e := ch.Accept()
					if e != nil {
						return
					}
					p.wg.Add(1)
					go func() {
						defer p.wg.Done()
						defer channel.Close()
						for req := range reqs {
							if req.Type == "pty-req" {
								p.ptys.Add(1)
							}
							if req.Type != "exec" {
								req.Reply(false, nil)
								continue
							}
							var command struct{ Command string }
							ssh.Unmarshal(req.Payload, &command)
							if command.Command == "blocked" {
								io.Copy(io.Discard, channel)
								return
							}
							if command.Command != "fixed" {
								req.Reply(false, nil)
								return
							}
							p.execs.Add(1)
							req.Reply(true, nil)
							handle(channel)
							return
						}
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		for _, c := range connections {
			c.Close()
		}
		mu.Unlock()
		p.wg.Wait()
	})
	return p
}

func exitPeer(ch ssh.Channel, code uint32) {
	ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{code}))
}

func TestSSHTransferAndReuse(t *testing.T) {
	payload := bytes.Repeat([]byte("日本語"), 230000)
	received := make(chan []byte, 2)
	peer := startPeer(t, func(ch ssh.Channel) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); ch.Write(bytes.Repeat([]byte("a"), CaptureLimit+12345)) }()
		go func() { defer wg.Done(); ch.Stderr().Write(bytes.Repeat([]byte("b"), CaptureLimit+7)) }()
		// Tiny reads deliberately split UTF-8 sequences.
		var all bytes.Buffer
		buf := make([]byte, 7)
		for {
			n, e := ch.Read(buf)
			all.Write(buf[:n])
			if e != nil {
				break
			}
		}
		received <- all.Bytes()
		wg.Wait()
		exitPeer(ch, 7)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, e := (SSHConnector{}).Connect(ctx, peer.host)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	cancel() // Connect context no longer owns the connection.
	for range 2 {
		run, stop := context.WithTimeout(context.Background(), 10*time.Second)
		r, e := s.Run(run, Request{Command: "fixed", Stdin: payload})
		stop()
		if e != nil || r.Communication != CommunicationComplete || r.ExitCode == nil || *r.ExitCode != 7 {
			t.Fatalf("%+v %v", r, e)
		}
		if len(r.Stdout) != CaptureLimit || len(r.Stderr) != CaptureLimit || !r.StdoutTruncated || !r.StderrTruncated {
			t.Fatal("capture limits")
		}
		if !bytes.Equal(<-received, payload) {
			t.Fatal("stdin changed")
		}
	}
	if peer.execs.Load() != 2 || peer.ptys.Load() != 0 {
		t.Fatal("exec/PTY requests")
	}
}

func TestSSHRejectKeysAndAuthentication(t *testing.T) {
	for _, mode := range []string{"unknown", "changed", "auth", "missing_known", "invalid_key", "encrypted", "wrong_passphrase", "missing_passphrase"} {
		t.Run(mode, func(t *testing.T) {
			p := startPeer(t, func(ch ssh.Channel) { io.Copy(io.Discard, ch); exitPeer(ch, 0) })
			want := "host_key_failed"
			switch mode {
			case "unknown":
				want = "host_key_unknown"
				os.WriteFile(p.host.KnownHostsFile, nil, 0600)
			case "changed":
				want = "host_key_changed"
				k, _ := testKey(t)
				address := net.JoinHostPort(p.host.Address, strconv.Itoa(int(p.host.Port)))
				os.WriteFile(p.host.KnownHostsFile, []byte(knownhosts.Line([]string{knownhosts.Normalize(address)}, k.PublicKey())+"\n"), 0600)
			case "missing_known":
				os.Remove(p.host.KnownHostsFile)
			case "auth":
				_, k := testKey(t)
				os.WriteFile(p.host.PrivateKeyFile, k, 0600)
				want = "authentication_failed"
			case "invalid_key":
				os.WriteFile(p.host.PrivateKeyFile, []byte("SECRET"), 0600)
				want = "authentication_failed"
			case "encrypted", "wrong_passphrase", "missing_passphrase":
				raw, _ := os.ReadFile(p.host.PrivateKeyFile)
				key, e := ssh.ParseRawPrivateKey(raw)
				if e != nil {
					t.Fatal(e)
				}
				block, e := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte("SECRET"))
				if e != nil {
					t.Fatal(e)
				}
				os.WriteFile(p.host.PrivateKeyFile, pem.EncodeToMemory(block), 0600)
				if mode == "encrypted" {
					t.Setenv("WINTABLE_KEY_PASSPHRASE", "SECRET")
				} else {
					t.Setenv("WINTABLE_KEY_PASSPHRASE", "wrong")
					if mode == "missing_passphrase" {
						t.Setenv("WINTABLE_KEY_PASSPHRASE", "")
					}
					want = "authentication_failed"
				}
			}
			before, _ := os.ReadFile(p.host.KnownHostsFile)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			s, e := (SSHConnector{}).Connect(ctx, p.host)
			if mode == "encrypted" {
				if e != nil {
					t.Fatal(e)
				}
				s.Close()
				return
			}
			if e == nil {
				s.Close()
				t.Fatal("accepted invalid credentials")
			}
			if e.Error() != want {
				t.Fatalf("unsafe or wrong error: %v", e)
			}
			after, _ := os.ReadFile(p.host.KnownHostsFile)
			if !bytes.Equal(before, after) || p.execs.Load() != 0 {
				t.Fatal("modified trust or executed")
			}
		})
	}
}

func TestSSHHandshakeCancellation(t *testing.T) {
	p := startPeer(t, func(ssh.Channel) {})
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, e := listener.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		io.Copy(io.Discard, c)
	}()
	p.host.Port = uint16(listener.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	s, e := (SSHConnector{}).Connect(ctx, p.host)
	if s != nil || e == nil || e.Error() != "deadline_exceeded" || time.Since(start) > 2*time.Second {
		t.Fatalf("%v %v", s, e)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handshake leaked connection")
	}
}

func TestSSHExecRejected(t *testing.T) {
	p := startPeer(t, func(ssh.Channel) { t.Error("unexpected exec") })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s, e := (SSHConnector{}).Connect(ctx, p.host)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r, e := s.Run(ctx, Request{Command: "rejected"})
	if e == nil || r.Communication == CommunicationComplete || r.ExitCode != nil {
		t.Fatal(r, e)
	}
}

func TestSSHDeadlineAndDisconnect(t *testing.T) {
	for _, mode := range []string{"blocked_exec", "blocked_send", "output_wait", "disconnect", "close"} {
		t.Run(mode, func(t *testing.T) {
			p := startPeer(t, func(ch ssh.Channel) {
				if mode == "disconnect" {
					return
				}
				if mode == "output_wait" {
					io.Copy(io.Discard, ch)
				}
				// Wait for client cancellation/Close to release the channel.
				buf := make([]byte, 1)
				for {
					if _, e := ch.Stderr().Write(buf); e != nil {
						return
					}
					time.Sleep(time.Millisecond)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			s, e := (SSHConnector{}).Connect(ctx, p.host)
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			run, stop := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer stop()
			if mode == "close" {
				go func() { time.Sleep(20 * time.Millisecond); s.Close(); s.Close() }()
			}
			start := time.Now()
			command := "fixed"
			if mode == "blocked_exec" {
				command = "blocked"
			}
			r, e := s.Run(run, Request{Command: command, Stdin: bytes.Repeat([]byte("x"), 4<<20)})
			if e == nil || r.Communication == CommunicationComplete || r.ExitCode != nil || time.Since(start) > 2*time.Second {
				t.Fatalf("%v %+v", e, r)
			}
			wantExecs := int32(1)
			if mode == "blocked_exec" {
				wantExecs = 0
			}
			if p.execs.Load() != wantExecs {
				t.Fatal("retried")
			}
		})
	}
}

func TestBoundedOutputBoundary(t *testing.T) {
	for _, n := range []int{CaptureLimit - 1, CaptureLimit, CaptureLimit + 1} {
		var b boundedOutput
		p := bytes.Repeat([]byte("x"), n)
		count, e := b.Write(p)
		if count != n || e != nil || len(b.data) != min(n, CaptureLimit) || b.truncated != (n > CaptureLimit) {
			t.Fatal(n)
		}
	}
}

func TestWrapperEnvelopeBoundary(t *testing.T) {
	for _, n := range []int{WrapperResponseLimit - 1, WrapperResponseLimit, WrapperResponseLimit + 1} {
		b := boundedOutput{limit: WrapperResponseLimit}
		p := bytes.Repeat([]byte("x"), n)
		count, e := b.Write(p)
		if count != n || e != nil || len(b.data) != min(n, WrapperResponseLimit) || b.truncated != (n > WrapperResponseLimit) {
			t.Fatal(n)
		}
	}
}
