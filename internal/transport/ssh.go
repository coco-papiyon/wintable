package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/coco-papiyon/wintable/internal/result"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHConnector uses only file-based public key authentication and mandatory
// known_hosts verification. It never enrolls keys or retries a connection.
type SSHConnector struct{}

func contextError(ctx context.Context, fallback string) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result.PublicError("deadline_exceeded")
	}
	if ctx.Err() != nil {
		return result.PublicError("cancelled")
	}
	return result.PublicError(fallback)
}

// watch joins its cancellation callback before returning, so a completed connect
// context cannot subsequently close a connection transferred to the caller.
func watch(ctx context.Context, closeConn func()) func() {
	done, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			closeConn()
		case <-done:
		}
	}()
	return func() { close(done); <-joined }
}

func (SSHConnector) Connect(ctx context.Context, h Host) (Session, error) {
	key, err := os.ReadFile(h.PrivateKeyFile)
	if err != nil {
		return nil, result.PublicError("authentication_failed")
	}
	signer, err := ssh.ParsePrivateKey(key)
	var encrypted *ssh.PassphraseMissingError
	if errors.As(err, &encrypted) {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(os.Getenv("WINTABLE_KEY_PASSPHRASE")))
	}
	if err != nil {
		return nil, result.PublicError("authentication_failed")
	}
	verify, err := knownhosts.New(h.KnownHostsFile)
	if err != nil {
		return nil, result.PublicError("host_key_failed")
	}
	hostCode := ""
	verified := false
	callback := func(host string, addr net.Addr, key ssh.PublicKey) error {
		err := verify(host, addr, key)
		if err != nil {
			hostCode = "host_key_failed"
			var mismatch *knownhosts.KeyError
			if errors.As(err, &mismatch) {
				if len(mismatch.Want) == 0 {
					hostCode = "host_key_unknown"
				} else {
					hostCode = "host_key_changed"
				}
			}
		}
		verified = err == nil
		return err
	}
	address := net.JoinHostPort(h.Address, strconv.Itoa(int(h.Port)))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, contextError(ctx, "connection_failed")
	}
	stop := watch(ctx, func() { _ = conn.Close() })
	c, chans, reqs, err := ssh.NewClientConn(conn, address, &ssh.ClientConfig{User: h.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: callback})
	stop()
	if err != nil || ctx.Err() != nil {
		_ = conn.Close()
		code := "connection_failed"
		if hostCode != "" {
			code = hostCode
		} else if verified {
			code = "authentication_failed"
		}
		return nil, contextError(ctx, code)
	}
	return &sshSession{client: ssh.NewClient(c, chans, reqs)}, nil
}

type sshSession struct {
	client *ssh.Client
	once   sync.Once
	run    sync.Mutex
}

func (s *sshSession) Close() error { s.once.Do(func() { _ = s.client.Close() }); return nil }

const CaptureLimit = 1 << 20
const WrapperResponseLimit = 3 << 20

type boundedOutput struct {
	data      []byte
	truncated bool
	limit     int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	limit := b.limit
	if limit == 0 {
		limit = CaptureLimit
	}
	keep := limit - len(b.data)
	if len(p) > keep {
		b.truncated = true
		p = p[:keep]
	}
	b.data = append(b.data, p...)
	return n, nil
}

func (s *sshSession) Run(ctx context.Context, req Request) (Response, error) {
	s.run.Lock()
	defer s.run.Unlock()
	response := Response{Communication: CommunicationInterrupted}
	if req.StdoutLimit < 0 || req.StdoutLimit > WrapperResponseLimit {
		return response, result.PublicError("internal_error")
	}
	if ctx.Err() != nil {
		return response, contextError(ctx, "connection_lost")
	}
	stop := watch(ctx, func() { _ = s.Close() })
	defer stop()
	task, err := s.client.NewSession()
	if err != nil {
		return response, contextError(ctx, "connection_lost")
	}
	defer task.Close()
	stdin, err := task.StdinPipe()
	if err != nil {
		return response, result.PublicError("connection_lost")
	}
	var stdout, stderr boundedOutput
	stdout.limit = req.StdoutLimit
	task.Stdout = &stdout
	task.Stderr = &stderr
	// Start requests exec only. No PTY, shell, environment or task interpolation.
	if err = task.Start(req.Command); err != nil {
		_ = s.Close()
		return response, contextError(ctx, "connection_lost")
	}
	sent := make(chan error, 1)
	go func() {
		_, e := io.Copy(stdin, bytes.NewReader(req.Stdin))
		eof := stdin.Close()
		if e == nil {
			e = eof
		}
		if e != nil {
			_ = s.Close()
		}
		sent <- e
	}()
	err = task.Wait() // SSH's independent stdout/stderr copy loops drain concurrently.
	// A peer may exit without reading stdin. Closing the channel releases writers.
	_ = task.Close()
	sendErr := <-sent
	response.Stdout, response.Stderr = stdout.data, stderr.data
	response.StdoutTruncated, response.StderrTruncated = stdout.truncated, stderr.truncated
	if ctx.Err() != nil {
		return response, contextError(ctx, "connection_lost")
	}
	var exit *ssh.ExitError
	if sendErr != nil || (err != nil && !errors.As(err, &exit)) {
		_ = s.Close()
		return response, result.PublicError("connection_lost")
	}
	code := 0
	if exit != nil {
		code = exit.ExitStatus()
	}
	response.ExitCode = &code
	response.Communication = CommunicationComplete
	return response, nil
}
