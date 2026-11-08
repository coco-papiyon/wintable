package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClassifiedTransportErrors(t *testing.T) {
	for _, phase := range []string{"connect", "run"} {
		for _, tc := range []struct {
			name string
			err  error
			code string
			exit int
		}{
			{"authentication", result.PublicError("authentication_failed"), "authentication_failed", 1},
			{"host_key", result.PublicError("host_key_failed"), "host_key_failed", 1},
			{"unknown_host_key", result.PublicError("host_key_unknown"), "host_key_unknown", 1},
			{"changed_host_key", result.PublicError("host_key_changed"), "host_key_changed", 1},
			{"internal", result.PublicError("internal_error"), "internal_error", 3},
			{"wrapped", fmt.Errorf("SECRET: %w", result.PublicError("host_key_failed")), "host_key_failed", 1},
			{"unknown_code", &result.Error{Code: "SECRET"}, "internal_error", 3},
			{"raw", errors.New("SECRET"), map[string]string{"connect": "connection_failed", "run": "connection_lost"}[phase], 1},
		} {
			t.Run(phase+"/"+tc.name, func(t *testing.T) {
				p, r := fixture(t, 2)
				closed, runs := 0, 0
				r.Connector = connectorFunc(func(_ context.Context, h transport.Host) (transport.Session, error) {
					if h.Name == "h000" && phase == "connect" {
						return nil, tc.err
					}
					return &session{run: func(context.Context, transport.Request) (transport.Response, error) {
						runs++
						if h.Name == "h000" {
							return transport.Response{}, tc.err
						}
						code := 0
						return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code}, nil
					}, close: func() error { closed++; return nil }}, nil
				})
				ctx := context.Background()
				rows, err := r.Execute(ctx, p, 1, time.Second, false)
				wantStatus, wantRuns, wantClosed := result.Unreachable, 3, 1
				if phase == "run" {
					wantStatus, wantRuns, wantClosed = result.Failed, 4, 2
				}
				if err != nil || len(rows) != 6 || rows[0].Status != wantStatus || rows[0].Error.Code != tc.code || runs != wantRuns || closed != wantClosed {
					t.Fatalf("rows=%+v err=%v runs=%d closed=%d", rows, err, runs, closed)
				}
				if (phase == "run") != (rows[0].Changed == nil) || rows[1].Status != result.Skipped || rows[2].Status != result.Skipped || rows[5].Status != result.Changed {
					t.Fatal(rows)
				}
				for _, format := range []string{"json", "text"} {
					var out, diag bytes.Buffer
					if exit := result.Finish(ctx, &out, &diag, rows, format, false, err); exit != tc.exit || strings.Contains(out.String()+diag.String(), "SECRET") {
						t.Fatalf("exit=%d output=%s diagnostic=%s", exit, out.String(), diag.String())
					}
					cancelled, cancel := context.WithCancel(ctx)
					cancel()
					if exit := result.Finish(cancelled, &out, &diag, rows, format, false, err); exit != 130 {
						t.Fatal(exit)
					}
				}
			})
		}
	}
}

type connectorFunc func(context.Context, transport.Host) (transport.Session, error)

func (f connectorFunc) Connect(c context.Context, h transport.Host) (transport.Session, error) {
	return f(c, h)
}

type session struct {
	run   func(context.Context, transport.Request) (transport.Response, error)
	close func() error
}

func (s *session) Run(c context.Context, r transport.Request) (transport.Response, error) {
	return s.run(c, r)
}
func (s *session) Close() error { return s.close() }
func fixture(t *testing.T, n int) (*Plan, Runtime) {
	t.Helper()
	payload, e := module.Prepare("a", 1, "command", map[string]any{"executable": `C:\test.exe`, "argv": []any{"日本語"}})
	if e != nil {
		t.Fatal(e)
	}
	p := &Plan{}
	for i := n - 1; i >= 0; i-- {
		name := fmt.Sprintf("h%03d", i)
		p.Hosts = append(p.Hosts, HostPlan{Name: name, Connection: transport.Host{Name: name}, Tasks: []TaskPlan{{"same", "command", payload, nil}, {"same", "command", payload, nil}, {"same", "command", payload, nil}}})
	}
	return p, Runtime{Wrappers: map[string]string{"command": "fixed"}, Decode: func(_ TaskPlan, r transport.Response) (result.Status, *bool, *result.Error) {
		b := true
		if *r.ExitCode != 0 {
			return result.Failed, nil, result.PublicError("task_failed")
		}
		return result.Changed, &b, nil
	}}
}
func TestParallelFailure(t *testing.T) {
	for _, forks := range []int{1, 3, 64} {
		t.Run(fmt.Sprint(forks), func(t *testing.T) {
			p, r := fixture(t, 8)
			var mu sync.Mutex
			active, max, closed := 0, 0, 0
			calls := map[string]int{}
			order := []string{}
			r.Connector = connectorFunc(func(ctx context.Context, h transport.Host) (transport.Session, error) {
				d, ok := ctx.Deadline()
				if !ok || time.Until(d) > ConnectionTimeout || time.Until(d) < 9*time.Second {
					t.Error("connection deadline")
				}
				mu.Lock()
				active++
				if active > max {
					max = active
				}
				order = append(order, h.Name)
				mu.Unlock()
				index := 0
				return &session{run: func(ctx context.Context, req transport.Request) (transport.Response, error) {
					if !bytes.Equal(req.Stdin, p.Hosts[0].Tasks[0].Payload.JSON()) || req.Command != "fixed" {
						t.Error("payload changed")
					}
					mu.Lock()
					calls[h.Name]++
					mu.Unlock()
					index++
					time.Sleep(time.Millisecond)
					code := 0
					if h.Name == "h001" && index == 2 {
						code = 1
					}
					return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code}, nil
				}, close: func() error { mu.Lock(); defer mu.Unlock(); active--; closed++; return nil }}, nil
			})
			rows, e := r.Execute(context.Background(), p, forks, time.Second, false)
			if e != nil || len(rows) != 24 || max > forks || closed != 8 || active != 0 {
				t.Fatalf("rows=%d max=%d closed=%d error=%v", len(rows), max, closed, e)
			}
			if calls["h001"] != 2 || rows[5].Status != result.Skipped {
				t.Fatal("failure did not stop host")
			}
			for h, n := range calls {
				if h != "h001" && n != 3 {
					t.Fatal(calls)
				}
			}
			if forks == 1 {
				for i, h := range order {
					if h != fmt.Sprintf("h%03d", i) {
						t.Fatal(order)
					}
				}
			}
		})
	}
}

func TestCancellationAndDeadline(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelParent), func(t *testing.T) {
			n := 1
			if cancelParent {
				n = 3
			}
			p, r := fixture(t, n)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closed := 0
			started := 0
			released := make(chan struct{})
			r.Connector = connectorFunc(func(context.Context, transport.Host) (transport.Session, error) {
				return &session{run: func(c context.Context, _ transport.Request) (transport.Response, error) {
					started++
					if cancelParent {
						cancel()
					}
					<-released
					return transport.Response{}, errors.New("secret")
				}, close: func() error { closed++; close(released); return nil }}, nil
			})
			rows, e := r.Execute(ctx, p, 1, time.Millisecond, false)
			if e != nil || closed != 1 || started != 1 || len(rows) != n*3 || rows[0].Changed != nil {
				t.Fatalf("%+v %v closed=%d", rows, e, closed)
			}
			if cancelParent {
				for _, row := range rows {
					if row.Status != result.Cancelled {
						t.Fatal(rows)
					}
				}
				if result.ExitCode(ctx, result.Counts{}) != 130 {
					t.Fatal("exit")
				}
			} else if rows[0].Status != result.Failed || rows[1].Status != result.Skipped {
				t.Fatal(rows)
			}
		})
	}
}

func TestConnectCleanup(t *testing.T) {
	for _, mode := range []string{"failure", "cancel", "close"} {
		t.Run(mode, func(t *testing.T) {
			p, r := fixture(t, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closed := 0
			r.Connector = connectorFunc(func(context.Context, transport.Host) (transport.Session, error) {
				s := &session{run: func(context.Context, transport.Request) (transport.Response, error) {
					code := 0
					return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code}, nil
				}, close: func() error {
					closed++
					if mode == "close" {
						return errors.New("secret")
					}
					return nil
				}}
				if mode == "cancel" {
					cancel()
				}
				if mode == "failure" {
					return s, errors.New("secret")
				}
				return s, nil
			})
			rows, e := r.Execute(ctx, p, 1, time.Second, false)
			if closed != 1 {
				t.Fatal(closed)
			}
			switch mode {
			case "failure":
				if rows[0].Status != result.Unreachable || *rows[0].Changed {
					t.Fatal(rows)
				}
			case "cancel":
				if rows[0].Status != result.Cancelled || *rows[0].Changed {
					t.Fatal(rows)
				}
			case "close":
				if e == nil || e.Error() != "internal_error" || rows[0].Status != result.Changed {
					t.Fatal(rows, e)
				}
			}
		})
	}
}

func TestTransportFailuresAndFreshDeadlines(t *testing.T) {
	for _, mode := range []string{"connect_deadline", "lost", "success"} {
		t.Run(mode, func(t *testing.T) {
			p, r := fixture(t, 1)
			closed, runs := 0, 0
			r.Connector = connectorFunc(func(c context.Context, _ transport.Host) (transport.Session, error) {
				if mode == "connect_deadline" {
					return nil, context.DeadlineExceeded
				}
				return &session{run: func(ctx context.Context, _ transport.Request) (transport.Response, error) {
					if c.Err() == nil {
						t.Error("connect timer not released")
					}
					d, ok := ctx.Deadline()
					if !ok || time.Until(d) < 90*time.Millisecond {
						t.Error("task deadline reused")
					}
					runs++
					if mode == "lost" {
						return transport.Response{}, errors.New("secret")
					}
					time.Sleep(15 * time.Millisecond)
					code := 0
					return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code}, nil
				}, close: func() error { closed++; return nil }}, nil
			})
			rows, e := r.Execute(context.Background(), p, 1, 100*time.Millisecond, false)
			if e != nil || len(rows) != 3 {
				t.Fatal(rows, e)
			}
			switch mode {
			case "connect_deadline":
				if rows[0].Error.Code != "deadline_exceeded" || rows[0].Status != result.Unreachable || runs != 0 {
					t.Fatal(rows)
				}
			case "lost":
				if rows[0].Changed != nil || rows[0].Error.Code != "connection_lost" || runs != 1 || closed != 1 {
					t.Fatal(rows)
				}
			case "success":
				if runs != 3 || closed != 1 {
					t.Fatal(rows)
				}
			}
		})
	}
}
