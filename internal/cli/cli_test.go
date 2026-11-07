package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"io"
	"os"
	"path/filepath"
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
	validFiles(t)
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "context marker")
	for _, n := range []string{"1", "64"} {
		args := []string{"run", "--inventory", "i", "--tasks", "t", "--limit", "win01,win01", "--forks", n, "--timeout", "1ms", "--format", "json", "--vars-file", "v", "--include-output"}
		called := false
		code := Run(ctx, args, io.Discard, io.Discard, func(gotCtx context.Context, got Options) int {
			called = true
			forks := 1
			if n == "64" {
				forks = 64
			}
			want := Options{Inventory: "i", Tasks: "t", Limit: "win01,win01", Forks: forks, Timeout: time.Millisecond, Format: "json", VarsFile: "v", IncludeOutput: true}
			want.Inventory, _ = filepath.Abs(want.Inventory)
			want.Tasks, _ = filepath.Abs(want.Tasks)
			want.VarsFile, _ = filepath.Abs(want.VarsFile)
			if got.Config == nil {
				t.Fatal("missing validated config")
			}
			got.Config = nil
			if got.Plan == nil {
				t.Fatal("missing validated plan")
			}
			got.Plan = nil
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
		want.Inventory, _ = filepath.Abs(want.Inventory)
		want.Tasks, _ = filepath.Abs(want.Tasks)
		o.Config = nil
		o.Plan = nil
		if o != want {
			t.Fatalf("defaults: %+v", o)
		}
		return 0
	})
}

func TestCancellation(t *testing.T) {
	validFiles(t)
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
	validFiles(t)
	var out, errOut bytes.Buffer
	code := Run(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t"}, &out, &errOut, nil)
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

func validFiles(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	for name, data := range map[string]string{
		"i": `{"version":1,"hosts":{"win01":{"address":"localhost","user":"user","private_key_file":"key","known_hosts_file":"known"}}}`,
		"t": `{"version":1,"hosts":"all","tasks":[{"name":"test","module":"command","args":{"executable":"C:\\Windows\\whoami.exe","argv":[]}}]}`,
		"v": `{}`,
	} {
		if err := os.WriteFile(name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFilesRejectedBeforeExecutor(t *testing.T) {
	for _, name := range []string{"i", "t", "v"} {
		t.Run(name, func(t *testing.T) {
			validFiles(t)
			if err := os.WriteFile(name, []byte(`{"SECRET":"secret","SECRET":"secret"}`), 0600); err != nil {
				t.Fatal(err)
			}
			var out, diag bytes.Buffer
			calls := 0
			code := Run(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t", "--vars-file", "v"}, &out, &diag, func(context.Context, Options) int { calls++; return 0 })
			if code != 2 || calls != 0 || out.Len() != 0 || !strings.Contains(diag.String(), "$") || strings.Contains(diag.String(), "secret") || strings.Contains(diag.String(), "SECRET") {
				t.Fatalf("%d %d %s", code, calls, diag.String())
			}
		})
	}
}

func TestV01NeverExecutes(t *testing.T) {
	for _, file := range []string{"i", "t", "v"} {
		for _, body := range []string{"", "version: 1", "[]", "{} {}", "\xff", "\xef\xbb\xbf\xef\xbb\xbf{}", `{"SECRET":1,"SECRET":2}`, `{"version":"1"}`, `{"version":2}`, `{"version":1,"unknown":"secret"}`, `{"version":1,"hosts":"all","tasks":[{"name":"secret","module":"unknown","args":{}}]}`, `{"version":1,"hosts":"all","tasks":[{"name":"secret","module":"command","args":{"executable":"C:\\a.exe","argv":[],"unknown":true}}]}`, `{"version":1,"hosts":"all","tasks":[{"name":"secret","module":"command","args":{"executable":"C:\\a.exe"}}]}`} {
			t.Run(file+body, func(t *testing.T) {
				if file == "v" && body == `{"version":"1"}` {
					t.Skip("valid string dictionary")
				}
				validFiles(t)
				if err := os.WriteFile(file, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				var out, diag bytes.Buffer
				calls := 0
				code := Run(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t", "--vars-file", "v"}, &out, &diag, func(context.Context, Options) int { calls++; return 0 })
				if code != 2 || calls != 0 || out.Len() != 0 || !strings.Contains(diag.String(), "$") || strings.Contains(diag.String(), "secret") || strings.Contains(diag.String(), "SECRET") {
					t.Fatalf("%d %d %s", code, calls, diag.String())
				}
			})
		}
	}
}

func TestDiagnosticFailurePriority(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"run"}} {
		if got := Run(context.Background(), args, brokenWriter{}, brokenWriter{}, nil); got != 3 {
			t.Fatal(args, got)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := Run(ctx, args, brokenWriter{}, brokenWriter{}, nil); got != 130 {
			t.Fatal(args, got)
		}
	}
}

func TestResultExecutorBoundary(t *testing.T) {
	validFiles(t)
	for _, format := range []string{"text", "json"} {
		var out, diag bytes.Buffer
		code := RunResults(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t", "--format", format}, &out, &diag, func(context.Context, Options) ([]result.TaskResult, error) {
			return []result.TaskResult{{Host: "a", Status: result.Failed}}, nil
		})
		if code != 1 || out.Len() == 0 || diag.Len() != 0 {
			t.Fatal(code, out.String(), diag.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if got := Run(ctx, []string{"run", "--inventory", "i", "--tasks", "t"}, io.Discard, io.Discard, func(context.Context, Options) int { cancel(); return 0 }); got != 130 {
		t.Fatal(got)
	}
}

func TestLastHostLastTaskFailsBeforeExecutor(t *testing.T) {
	for _, value := range []string{"${missing}", "relative", strings.Repeat("s", module.ContentLimit+1)} {
		t.Run("last", func(t *testing.T) {
			validFiles(t)
			inv := map[string]any{"version": 1, "hosts": map[string]any{}}
			for _, h := range []string{"a", "z"} {
				v := "ok"
				if h == "z" {
					v = value
				}
				inv["hosts"].(map[string]any)[h] = map[string]any{"address": "localhost", "user": "SECRET", "private_key_file": "key", "known_hosts_file": "known", "vars": map[string]any{"v": v}}
			}
			tasks := map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "first", "module": "file_content", "args": map[string]any{"path": `C:\a`, "content": "ok"}}, map[string]any{"name": "${not_expanded}", "module": "file_content", "args": map[string]any{"path": `C:\a`, "content": "${v}"}}}}
			if value == "relative" {
				tasks["tasks"].([]any)[1].(map[string]any)["args"] = map[string]any{"path": "${v}", "content": "ok"}
				inv["hosts"].(map[string]any)["a"].(map[string]any)["vars"] = map[string]any{"v": `C:\ok`}
			}
			// A variable containing another reference is deliberately non-recursive;
			// undefined lookup must occur directly in the last task instead.
			if value == "${missing}" {
				delete(inv["hosts"].(map[string]any)["z"].(map[string]any)["vars"].(map[string]any), "v")
			}
			for name, obj := range map[string]any{"i": inv, "t": tasks} {
				data, e := json.Marshal(obj)
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(name, data, 0600); e != nil {
					t.Fatal(e)
				}
			}
			var diag bytes.Buffer
			calls := 0
			code := Run(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t"}, io.Discard, &diag, func(context.Context, Options) int { calls++; return 0 })
			if code != 2 || calls != 0 || strings.Contains(diag.String(), "SECRET") {
				t.Fatalf("%d %d %s", code, calls, diag.String())
			}
		})
	}
}

func TestWrapperCheckedBeforeExecution(t *testing.T) {
	validFiles(t)
	for _, n := range []int{7999, 8000, 8001} {
		calls := 0
		code := RunWithWrappers(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t"}, io.Discard, io.Discard, func(context.Context, Options) int { calls++; return 0 }, []string{strings.Repeat("x", n)})
		if n <= 8000 {
			if code != 0 || calls != 1 {
				t.Fatal(n, code, calls)
			}
		} else if code != 3 || calls != 0 {
			t.Fatal(n, code, calls)
		}
	}
}
