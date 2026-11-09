package transport

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/coco-papiyon/wintable/internal/result"
	"golang.org/x/crypto/ssh"
)

func (SSHConnector) SupportsStream() bool { return true }

func (s *sshSession) Stream(ctx context.Context, req Request, consume func(io.Reader) error) (Response, error) {
	s.run.Lock()
	defer s.run.Unlock()
	response := Response{Communication: CommunicationInterrupted}
	if consume == nil {
		return response, result.PublicError("internal_error")
	}
	if ctx.Err() != nil {
		return response, contextError(ctx, "connection_lost")
	}
	stop := watch(ctx, func() { _ = s.Close() })
	defer stop()
	task, e := s.client.NewSession()
	if e != nil {
		return response, contextError(ctx, "connection_lost")
	}
	defer task.Close()
	stdin, e := task.StdinPipe()
	if e != nil {
		return response, result.PublicError("connection_lost")
	}
	stdout, e := task.StdoutPipe()
	if e != nil {
		return response, result.PublicError("connection_lost")
	}
	task.Stderr = io.Discard
	if e = task.Start(req.Command); e != nil {
		_ = s.Close()
		return response, contextError(ctx, "connection_lost")
	}
	sent := make(chan error, 1)
	go func() {
		_, e := io.Copy(stdin, bytes.NewReader(req.Stdin))
		end := stdin.Close()
		if e == nil {
			e = end
		}
		if e != nil {
			_ = s.Close()
		}
		sent <- e
	}()
	consumed := consume(stdout)
	if consumed != nil {
		_ = s.Close()
	}
	e = task.Wait()
	_ = task.Close()
	sendErr := <-sent
	if ctx.Err() != nil {
		return response, contextError(ctx, "connection_lost")
	}
	if consumed != nil {
		return response, result.PublicError("task_failed")
	}
	var exit *ssh.ExitError
	if sendErr != nil || (e != nil && !errors.As(e, &exit)) {
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
