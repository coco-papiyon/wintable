package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}, {"run", "--help"}, {"run", "-h"}} {
		var out, errOut bytes.Buffer
		code := Run(context.Background(), args, &out, &errOut, func(context.Context, Options) int { t.Fatal("executed help"); return 0 })
		if code != 0 || errOut.Len() != 0 {
			t.Fatalf("%v: %d %s", args, code, errOut.String())
		}
		for _, s := range []string{"default: 5", "default: 60s", "default: text", "default: false", "Windows 11", "known_hosts", "cmd.exe", "JSON v1", "secrets", "masking", "exit 3"} {
			if !strings.Contains(out.String(), s) {
				t.Errorf("help missing %q", s)
			}
		}
	}
}

func TestInvalidInputNeverExecutes(t *testing.T) {
	base := []string{"run", "--inventory", "i.json", "--tasks", "t.json"}
	cases := [][]string{nil, {"unknown"}, {"run"}, {"help", "extra"}, {"run", "--inventory", "i"}, {"run", "--tasks", "t"}}
	for _, tail := range [][]string{
		{"--unknown"}, {"--check"}, {"--become"}, {"--retry", "1"}, {"--forks", "0"}, {"--forks", "65"}, {"--forks", "-1"}, {"--forks", "bad"},
		{"--timeout", "0s"}, {"--timeout", "-1s"}, {"--timeout", "bad"}, {"--format", "yaml"}, {"--format", ""}, {"--include-output=bad"},
		{"--inventory", ""}, {"--tasks", " "}, {"extra"}, {"--", "extra"}, {"--inventory"},
	} {
		cases = append(cases, append(append([]string{}, base...), tail...))
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := Run(context.Background(), args, &out, &errOut, func(context.Context, Options) int { t.Fatal("executed invalid input"); return 0 }); code != 2 || out.Len() != 0 || errOut.Len() == 0 {
				t.Fatalf("code/output: %d %q %q", code, out.String(), errOut.String())
			}
		})
	}
}

func TestOptionsAndContext(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "context marker")
	for _, n := range []string{"1", "64"} {
		args := []string{"run", "--inventory", "i", "--tasks", "t", "--limit", "win01,win02", "--forks", n, "--timeout", "1ms", "--format", "json", "--vars-file", "v", "--include-output"}
		called := false
		code := Run(ctx, args, io.Discard, io.Discard, func(gotCtx context.Context, got Options) int {
			called = true
			forks := 1
			if n == "64" {
				forks = 64
			}
			want := Options{Inventory: "i", Tasks: "t", Limit: "win01,win02", Forks: forks, Timeout: time.Millisecond, Format: "json", VarsFile: "v", IncludeOutput: true}
			if !reflect.DeepEqual(got, want) || gotCtx != ctx {
				t.Fatalf("got %+v, ctx %v", got, gotCtx)
			}
			return 1
		})
		if code != 1 || !called {
			t.Fatalf("code %d called %v", code, called)
		}
	}
	Run(ctx, []string{"run", "--inventory", "i", "--tasks", "t"}, io.Discard, io.Discard, func(_ context.Context, o Options) int {
		want := Options{Inventory: "i", Tasks: "t", Forks: 5, Timeout: 60 * time.Second, Format: "text"}
		if o != want {
			t.Fatalf("defaults: %+v", o)
		}
		return 0
	})
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	args := []string{"run", "--inventory", "i", "--tasks", "t"}
	if code := Run(ctx, args, io.Discard, io.Discard, func(passed context.Context, _ Options) int {
		cancel()
		<-passed.Done()
		if !errors.Is(passed.Err(), context.Canceled) {
			t.Fatal(passed.Err())
		}
		return 130
	}); code != 130 {
		t.Fatal(code)
	}
	if code := Run(ctx, args, io.Discard, io.Discard, func(context.Context, Options) int { t.Fatal("executed cancelled"); return 0 }); code != 130 {
		t.Fatal(code)
	}
}

func TestFoundationDoesNotReportSuccess(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"run", "--inventory", "missing", "--tasks", "missing"}, &out, &errOut, nil)
	if code != 3 || out.Len() != 0 || !strings.Contains(errOut.String(), "execution_not_implemented") {
		t.Fatalf("%d %q %q", code, out.String(), errOut.String())
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("broken") }
func TestHelpOutputFailure(t *testing.T) {
	if Run(context.Background(), []string{"help"}, brokenWriter{}, io.Discard, nil) != 3 {
		t.Fatal("expected 3")
	}
}

func TestInvalidValueIsNotEchoed(t *testing.T) {
	var out bytes.Buffer
	Run(context.Background(), []string{"run", "--timeout=secret-value"}, io.Discard, &out, nil)
	if strings.Contains(out.String(), "secret-value") {
		t.Fatal("raw argument leaked")
	}
}
