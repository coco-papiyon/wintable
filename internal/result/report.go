package result

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const OutputLimit = 1 << 20

// PublicError is the only conversion boundary for lower-layer failures. Callers
// classify causes without copying error text; unknown codes become internal_error.
func PublicError(code string) *Error {
	switch code {
	case "input_error", "connection_failed", "authentication_failed", "host_key_failed", "task_failed", "deadline_exceeded", "connection_lost", "cancelled", "internal_error", "output_failed", "execution_not_implemented":
	default:
		code = "internal_error"
	}
	return &Error{Code: code}
}
func (e *Error) Error() string { return PublicError(e.Code).Code }

// Capture consumes every byte while retaining only the opted-in prefix.
// Use a separate Capture for each stream; it is owned by its reader goroutine.
type Capture struct {
	enabled   bool
	data      []byte
	truncated bool
}

func NewCapture(enabled bool) *Capture { return &Capture{enabled: enabled} }
func (c *Capture) Write(p []byte) (int, error) {
	n := len(p)
	if c.enabled {
		remaining := OutputLimit - len(c.data)
		if len(p) > remaining {
			c.truncated = true
			p = p[:remaining]
		}
		c.data = append(c.data, p...)
	}
	return n, nil
}
func (c *Capture) Output() (*string, bool) {
	if !c.enabled {
		return nil, false
	}
	s := base64.StdEncoding.EncodeToString(c.data)
	return &s, c.truncated
}

type Counts struct {
	OK          int `json:"ok"`
	Changed     int `json:"changed"`
	Failed      int `json:"failed"`
	Unreachable int `json:"unreachable"`
	Skipped     int `json:"skipped"`
	Cancelled   int `json:"cancelled"`
}

func (c *Counts) add(s Status) bool {
	switch s {
	case OK:
		c.OK++
	case Changed:
		c.Changed++
	case Failed:
		c.Failed++
	case Unreachable:
		c.Unreachable++
	case Skipped:
		c.Skipped++
	case Cancelled:
		c.Cancelled++
	default:
		return false
	}
	return true
}

type HostResult struct {
	Host   string       `json:"host"`
	Tasks  []TaskResult `json:"tasks"`
	Counts Counts       `json:"counts"`
}
type Report struct {
	Hosts  []HostResult `json:"hosts"`
	Counts Counts       `json:"counts"`
}

// Aggregate copies inputs; invalid internal results never produce partial output.
func Aggregate(tasks []TaskResult, includeOutput bool) (Report, error) {
	r := Report{Hosts: []HostResult{}}
	ordered := append([]TaskResult(nil), tasks...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Host != ordered[j].Host {
			return ordered[i].Host < ordered[j].Host
		}
		return ordered[i].TaskIndex < ordered[j].TaskIndex
	})
	for i, t := range ordered {
		if t.Host == "" || t.TaskIndex < 0 || t.DurationMS < 0 || !r.Counts.add(t.Status) || (i > 0 && ordered[i-1].Host == t.Host && ordered[i-1].TaskIndex == t.TaskIndex) {
			return Report{}, PublicError("internal_error")
		}
		if t.Error != nil {
			t.Error = PublicError(t.Error.Code)
		}
		if !includeOutput {
			t.StdoutBase64 = nil
			t.StderrBase64 = nil
			t.StdoutTruncated = false
			t.StderrTruncated = false
		} else {
			for _, stream := range []struct {
				body      **string
				truncated *bool
			}{{&t.StdoutBase64, &t.StdoutTruncated}, {&t.StderrBase64, &t.StderrTruncated}} {
				if *stream.body == nil {
					continue
				}
				capture := NewCapture(true)
				if _, err := io.Copy(capture, base64.NewDecoder(base64.StdEncoding, strings.NewReader(**stream.body))); err != nil {
					return Report{}, PublicError("internal_error")
				}
				body, truncated := capture.Output()
				*stream.body = body
				*stream.truncated = *stream.truncated || truncated
			}
		}
		if len(r.Hosts) == 0 || r.Hosts[len(r.Hosts)-1].Host != t.Host {
			r.Hosts = append(r.Hosts, HostResult{Host: t.Host, Tasks: []TaskResult{}})
		}
		h := &r.Hosts[len(r.Hosts)-1]
		h.Tasks = append(h.Tasks, t)
		h.Counts.add(t.Status)
	}
	return r, nil
}

// ExitCode applies the product priority once, including parent cancellation.
func ExitCode(ctx context.Context, counts Counts, codes ...int) int {
	code := 0
	if counts.Failed+counts.Unreachable > 0 {
		code = 1
	}
	rank := map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 130: 4}
	for _, c := range codes {
		if _, ok := rank[c]; !ok {
			c = 3
		}
		if rank[c] > rank[code] {
			code = c
		}
	}
	if counts.Cancelled > 0 || ctx.Err() != nil {
		return 130
	}
	return code
}

// WriteAll detects short writes, including writers that fail without an error.
func WriteAll(w io.Writer, b []byte) error {
	n, e := w.Write(b)
	if e != nil || n != len(b) {
		return PublicError("output_failed")
	}
	return nil
}

// Render builds a single document before touching the destination. A failed
// write can leave a partial document; callers must never append another JSON.
func Render(w io.Writer, tasks []TaskResult, format string, includeOutput bool) (Counts, error) {
	r, e := Aggregate(tasks, includeOutput)
	if e != nil {
		return Counts{}, e
	}
	var b []byte
	switch format {
	case "json":
		b, e = json.Marshal(r)
		b = append(b, '\n')
	case "text":
		for _, h := range r.Hosts {
			b = fmt.Appendf(b, "host=%q\n", h.Host)
			for _, t := range h.Tasks {
				changed := "null"
				if t.Changed != nil {
					changed = fmt.Sprint(*t.Changed)
				}
				exit := "null"
				if t.ExitCode != nil {
					exit = fmt.Sprint(*t.ExitCode)
				}
				b = fmt.Appendf(b, "  task_index=%d task=%q status=%s changed=%s exit_code=%s duration_ms=%d", t.TaskIndex, t.Task, t.Status, changed, exit, t.DurationMS)
				if t.Error != nil {
					b = fmt.Appendf(b, " error=%s", t.Error.Code)
				}
				b = append(b, '\n')
				if t.StdoutBase64 != nil {
					b = fmt.Appendf(b, "    stdout_base64=%s stdout_truncated=%t\n", *t.StdoutBase64, t.StdoutTruncated)
				}
				if t.StderrBase64 != nil {
					b = fmt.Appendf(b, "    stderr_base64=%s stderr_truncated=%t\n", *t.StderrBase64, t.StderrTruncated)
				}
			}
			b = fmt.Appendf(b, "  counts=%+v\n", h.Counts)
		}
		b = fmt.Appendf(b, "counts=%+v\n", r.Counts)
	default:
		return r.Counts, PublicError("internal_error")
	}
	if e != nil {
		return r.Counts, PublicError("internal_error")
	}
	return r.Counts, WriteAll(w, b)
}

// Finish is the common engine/CLI result boundary. Raw errors never reach a writer.
func Finish(ctx context.Context, stdout, stderr io.Writer, tasks []TaskResult, format string, includeOutput bool, failure error) int {
	counts, e := Render(stdout, tasks, format, includeOutput)
	codes := []int{}
	for _, task := range tasks {
		if task.Status == Cancelled {
			codes = append(codes, 130)
		}
		if task.Error != nil {
			switch PublicError(task.Error.Code).Code {
			case "internal_error", "output_failed", "execution_not_implemented":
				codes = append(codes, 3)
			case "input_error":
				codes = append(codes, 2)
			case "cancelled":
				codes = append(codes, 130)
			}
		}
	}
	for _, err := range []error{failure, e} {
		if err == nil {
			continue
		}
		safe := PublicError("internal_error")
		var classified *Error
		if errors.As(err, &classified) {
			safe = PublicError(classified.Code)
		}
		code := 3
		switch safe.Code {
		case "input_error":
			code = 2
		case "cancelled":
			code = 130
		case "connection_failed", "authentication_failed", "host_key_failed", "task_failed", "deadline_exceeded", "connection_lost":
			code = 1
		}
		codes = append(codes, code)
		if WriteAll(stderr, []byte(safe.Code+"\n")) != nil {
			codes = append(codes, 3)
		}
	}
	return ExitCode(ctx, counts, codes...)
}
