package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"golang.org/x/crypto/ssh"
	"io"
	"testing"
	"time"
)

func TestSSHStream(t *testing.T) {
	body := bytes.Repeat([]byte{0, 255, 13, 10}, 400000)
	peer := startPeer(t, func(ch ssh.Channel) {
		io.Copy(io.Discard, ch)
		ch.Stderr().Write(bytes.Repeat([]byte("secret diagnostic"), 100000))
		ch.Write(body)
		exitPeer(ch, 0)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, e := (SSHConnector{}).Connect(ctx, peer.host)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	for i := 0; i < 2; i++ {
		h := sha256.New()
		r, e := session.(StreamSession).Stream(ctx, Request{Command: "fixed", Stdin: []byte("request")}, func(r io.Reader) error { _, e := io.Copy(h, r); return e })
		if e != nil || r.ExitCode == nil || *r.ExitCode != 0 || r.Communication != CommunicationComplete || len(r.Stdout) != 0 || len(r.Stderr) != 0 || !bytes.Equal(h.Sum(nil), sha256sum(body)) {
			t.Fatal("stream failed", e, r)
		}
	}
	if peer.execs.Load() != 2 || peer.ptys.Load() != 0 {
		t.Fatal("retry or PTY")
	}
}
func sha256sum(b []byte) []byte { h := sha256.Sum256(b); return h[:] }
func TestSSHStreamStopsOnFailure(t *testing.T) {
	for _, mode := range []string{"consumer", "timeout", "cancel", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			release := make(chan struct{})
			peer := startPeer(t, func(ch ssh.Channel) {
				io.Copy(io.Discard, ch)
				if mode == "disconnect" {
					return
				}
				ch.Write([]byte("x"))
				<-release
			})
			t.Cleanup(func() { close(release) })
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			session, e := (SSHConnector{}).Connect(ctx, peer.host)
			if e != nil {
				t.Fatal(e)
			}
			defer session.Close()
			_, e = session.(StreamSession).Stream(ctx, Request{Command: "fixed"}, func(r io.Reader) error {
				if mode == "consumer" {
					return io.ErrClosedPipe
				}
				if mode == "cancel" {
					cancel()
				}
				_, e := io.Copy(io.Discard, r)
				return e
			})
			if e == nil {
				t.Fatal("failure accepted")
			}
			if (mode == "timeout" || mode == "cancel") && ctx.Err() == nil {
				t.Fatal("context interruption not exercised")
			}
			if peer.execs.Load() != 1 {
				t.Fatal("retried")
			}
		})
	}
}
