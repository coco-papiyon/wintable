package engine

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

const ConnectionTimeout = 10 * time.Second

// Runtime supplies fixed, prevalidated launch lines and module response semantics.
// Decode returns only module-owned status, changed and classified error fields.
// Engine owns identity, timing, transport errors, exit code and output capture.
type Runtime struct {
	Backends  module.Backends
	Connector transport.Connector
	Wrappers  map[string]string
	Decode    func(TaskPlan, transport.Response) (result.Status, *bool, *result.Error)
	// DecodeResult receives wrapper JSON and returns only module-owned fields.
	DecodeResult func(TaskPlan, transport.Response, bool) result.TaskResult
}

// Execute consumes an entirely validated plan. It never retries or reserializes
// payloads. Slots remain occupied until session cleanup has completed.
func (r Runtime) Execute(ctx context.Context, p *Plan, forks int, timeout time.Duration, output bool) ([]result.TaskResult, error) {
	if p == nil || forks < 1 || forks > 64 || timeout <= 0 || r.Connector == nil {
		return nil, result.PublicError("internal_error")
	}
	hosts := append([]HostPlan(nil), p.Hosts...)
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	for i, h := range hosts {
		if h.Name == "" || (i > 0 && hosts[i-1].Name == h.Name) {
			return nil, result.PublicError("internal_error")
		}
		for _, t := range h.Tasks {
			if t.Backend != nil {
				if t.Backend.Target != (module.Key{OS: h.OS, Module: t.Module}) || t.Backend.Check() != nil {
					return nil, result.PublicError("internal_error")
				}
			} else if (h.OS != "" && h.OS != "windows") || (r.Decode == nil && r.DecodeResult == nil) || module.ValidateWrapper(r.Wrappers[t.Module]) != nil {
				return nil, result.PublicError("internal_error")
			}
		}
	}
	rows := make([][]result.TaskResult, len(hosts))
	failures := make([]error, len(hosts))
	var wg sync.WaitGroup
	slots := make(chan struct{}, forks)
	for i, h := range hosts {
		acquired := false
		if ctx.Err() == nil {
			select {
			case slots <- struct{}{}:
				acquired = true
			case <-ctx.Done():
			}
		}
		if ctx.Err() != nil {
			rows[i] = unstarted(h, 0, result.Cancelled, "cancelled")
			if acquired {
				<-slots
			}
			continue
		}
		wg.Add(1)
		go func(i int, h HostPlan) {
			defer wg.Done()
			defer func() { <-slots }()
			rows[i], failures[i] = r.host(ctx, h, timeout, output)
		}(i, h)
	}
	wg.Wait()
	all := []result.TaskResult{}
	var failure error
	for i := range rows {
		all = append(all, rows[i]...)
		if failures[i] != nil {
			failure = failures[i]
		}
	}
	return all, failure
}

func unstarted(h HostPlan, start int, status result.Status, code string) []result.TaskResult {
	rows := []result.TaskResult{}
	for i := start; i < len(h.Tasks); i++ {
		b := false
		t := result.TaskResult{Host: h.Name, Task: h.Tasks[i].Name, TaskIndex: i, Status: status, Changed: &b}
		if code != "" {
			t.Error = result.PublicError(code)
		}
		rows = append(rows, t)
	}
	return rows
}

// transportError preserves explicit classifications, including wrapped errors,
// but never uses lower-layer error text as a public code.
func transportError(err error, fallback string) *result.Error {
	var classified *result.Error
	if errors.As(err, &classified) && classified != nil {
		return result.PublicError(classified.Code)
	}
	return result.PublicError(fallback)
}

func (r Runtime) host(ctx context.Context, h HostPlan, timeout time.Duration, output bool) (rows []result.TaskResult, failure error) {
	if ctx.Err() != nil {
		return unstarted(h, 0, result.Cancelled, "cancelled"), nil
	}
	if len(h.Tasks) == 0 {
		return nil, nil
	}
	connectCtx, cancel := context.WithTimeout(ctx, ConnectionTimeout)
	s, err := r.Connector.Connect(connectCtx, h.Connection)
	expired := connectCtx.Err() != nil || errors.Is(err, context.DeadlineExceeded)
	cancel()
	closeSession := func() {}
	if s != nil {
		var once sync.Once
		var closeErr error
		closeSession = func() { once.Do(func() { closeErr = s.Close() }) }
		done := make(chan struct{})
		watched := make(chan struct{})
		go func() {
			defer close(watched)
			select {
			case <-ctx.Done():
				closeSession()
			case <-done:
			}
		}()
		defer func() {
			close(done)
			closeSession()
			<-watched
			if closeErr != nil {
				failure = result.PublicError("internal_error")
			}
		}()
	}
	if ctx.Err() != nil {
		return unstarted(h, 0, result.Cancelled, "cancelled"), nil
	}
	if err != nil || expired || s == nil {
		code := "connection_failed"
		if expired {
			code = "deadline_exceeded"
		}
		rows = unstarted(h, 0, result.Skipped, "")
		rows[0].Status = result.Unreachable
		rows[0].Error = transportError(err, code)
		if expired {
			rows[0].Error = result.PublicError(code)
		}
		return rows, nil
	}
	for i, t := range h.Tasks {
		if ctx.Err() != nil {
			return append(rows, unstarted(h, i, result.Cancelled, "cancelled")...), nil
		}
		var request transport.Request
		if t.Backend != nil {
			request = t.Backend.Request(t.Payload)
		} else {
			request, _ = t.Payload.Request(r.Wrappers[t.Module])
		}
		taskCtx, stop := context.WithTimeout(ctx, timeout)
		done, watched := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(watched)
			select {
			case <-taskCtx.Done():
				closeSession()
			case <-done:
			}
		}()

		start := time.Now()
		response, runErr := s.Run(taskCtx, request)
		close(done)
		<-watched
		deadline := taskCtx.Err() != nil || errors.Is(runErr, context.DeadlineExceeded)
		stop()
		row := result.TaskResult{Host: h.Name, Task: t.Name, TaskIndex: i, DurationMS: time.Since(start).Milliseconds(), ExitCode: response.ExitCode}
		if r.DecodeResult != nil || t.Backend != nil {
			row.ExitCode = nil
		} // outer wrapper status is never a child exit code
		switch {
		case ctx.Err() != nil:
			row.Status = result.Cancelled
			row.Error = result.PublicError("cancelled")
		case deadline:
			row.Status = result.Failed
			row.Error = result.PublicError("deadline_exceeded")
		case runErr != nil || response.Communication != transport.CommunicationComplete:
			row.Status = result.Failed
			row.Error = transportError(runErr, "connection_lost")
		default:
			if r.DecodeResult != nil || t.Backend != nil {
				var decoded result.TaskResult
				if t.Backend != nil {
					decoded = t.Backend.Decode(response, output)
				} else {
					decoded = r.DecodeResult(t, response, output)
				}
				row.Status, row.Changed, row.Error, row.ExitCode = decoded.Status, decoded.Changed, decoded.Error, decoded.ExitCode
				row.StdoutBase64, row.StderrBase64 = decoded.StdoutBase64, decoded.StderrBase64
				row.StdoutTruncated, row.StderrTruncated = decoded.StdoutTruncated, decoded.StderrTruncated
			} else {
				row.Status, row.Changed, row.Error = r.Decode(t, response)
			}
			if row.Status != result.OK && row.Status != result.Changed && row.Status != result.Failed {
				row.Status = result.Failed
				row.Changed = nil
				row.Error = result.PublicError("internal_error")
			}
			if row.Error != nil {
				row.Error = result.PublicError(row.Error.Code)
			}
		}
		if output && r.DecodeResult == nil && t.Backend == nil {
			a, b := result.NewCapture(true), result.NewCapture(true)
			a.Write(response.Stdout)
			b.Write(response.Stderr)
			row.StdoutBase64, row.StdoutTruncated = a.Output()
			row.StderrBase64, row.StderrTruncated = b.Output()
			row.StdoutTruncated = row.StdoutTruncated || response.StdoutTruncated
			row.StderrTruncated = row.StderrTruncated || response.StderrTruncated
		}
		rows = append(rows, row)
		if row.Status == result.Cancelled {
			return append(rows, unstarted(h, i+1, result.Cancelled, "cancelled")...), nil
		}
		if row.Status == result.Failed {
			return append(rows, unstarted(h, i+1, result.Skipped, "")...), nil
		}
	}
	return rows, nil
}
