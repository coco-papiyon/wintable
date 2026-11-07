package result

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }
func fixture() []TaskResult {
	statuses := []Status{OK, Changed, Failed, Unreachable, Skipped, Cancelled}
	tasks := []TaskResult{}
	for i, s := range statuses {
		tasks = append(tasks, TaskResult{Host: []string{"z", "a"}[i%2], Task: "same", TaskIndex: i, Status: s, Changed: ptr(false), DurationMS: int64(i)})
	}
	tasks[1].Changed = ptr(true)
	tasks[1].ExitCode = ptr(0)
	tasks[2].Changed = nil
	return tasks
}
func TestReportContract(t *testing.T) {
	tasks := fixture()
	original := tasks[0]
	tasks[0].Error = &Error{Code: "SECRET private key passphrase"}
	tasks[0].StdoutBase64 = ptr("SECRET")
	tasks[0].StderrBase64 = ptr("SECRET")
	var out bytes.Buffer
	counts, e := Render(&out, tasks, "json", false)
	if e != nil {
		t.Fatal(e)
	}
	want := Counts{1, 1, 1, 1, 1, 1}
	if counts != want {
		t.Fatal(counts)
	}
	d := json.NewDecoder(&out)
	var r Report
	if e = d.Decode(&r); e != nil {
		t.Fatal(e)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		t.Fatal("multiple documents")
	}
	if r.Counts != want || len(r.Hosts) != 2 || r.Hosts[0].Host != "a" {
		t.Fatal(r)
	}
	sum := Counts{}
	for _, h := range r.Hosts {
		local := Counts{}
		last := -1
		for _, task := range h.Tasks {
			if task.TaskIndex <= last {
				t.Fatal("order")
			}
			last = task.TaskIndex
			local.add(task.Status)
			sum.add(task.Status)
		}
		if local != h.Counts {
			t.Fatal(h)
		}
	}
	if sum != want {
		t.Fatal(sum)
	}
	a := r.Hosts[1].Tasks
	if a[0].StdoutBase64 != nil || a[0].StderrBase64 != nil || a[0].Error.Code != "internal_error" || a[1].Changed != nil || a[2].Changed == nil || *a[2].Changed {
		t.Fatal(a)
	}
	if tasks[0].Host != original.Host || tasks[0].Error.Code != "SECRET private key passphrase" {
		t.Fatal("mutated")
	}
	for _, format := range []string{"json", "text"} {
		var first, second bytes.Buffer
		Render(&first, tasks, format, false)
		Render(&second, tasks, format, false)
		if first.String() != second.String() || strings.Contains(first.String(), "SECRET") {
			t.Fatal(first.String())
		}
	}
	var empty bytes.Buffer
	Render(&empty, nil, "json", false)
	if !strings.Contains(empty.String(), `"hosts":[]`) {
		t.Fatal(empty.String())
	}
}
func TestInvalidInternalResults(t *testing.T) {
	for _, tasks := range [][]TaskResult{{{Host: "a", Status: "secret"}}, {{Host: "a", Status: OK, TaskIndex: -1}}, {{Host: "a", Status: OK, DurationMS: -1}}, {{Status: OK}}, {{Host: "a", Status: OK}, {Host: "a", Status: OK}}} {
		var out bytes.Buffer
		if _, e := Render(&out, tasks, "json", false); e == nil || out.Len() != 0 {
			t.Fatal(tasks, e)
		}
	}
}
func TestCaptureBoundaries(t *testing.T) {
	for _, n := range []int{0, OutputLimit - 1, OutputLimit, OutputLimit + 1, 5 * OutputLimit} {
		raw := bytes.Repeat([]byte{0xff, 0, 0x81}, (n+2)/3)[:n]
		for _, enabled := range []bool{false, true} {
			c := NewCapture(enabled)
			copied, e := io.Copy(c, bytes.NewReader(raw))
			if e != nil || copied != int64(n) {
				t.Fatal(copied, e)
			}
			body, truncated := c.Output()
			if !enabled {
				if body != nil || truncated || len(c.data) != 0 {
					t.Fatal("retained")
				}
				continue
			}
			b, e := base64.StdEncoding.DecodeString(*body)
			limit := min(n, OutputLimit)
			if e != nil || !bytes.Equal(b, raw[:limit]) || truncated != (n > OutputLimit) {
				t.Fatal(n, e)
			}
			tasks := []TaskResult{{Host: "a", Status: OK, StdoutBase64: body, StdoutTruncated: truncated, StderrBase64: ptr("")}}
			for _, format := range []string{"json", "text"} {
				var out bytes.Buffer
				if _, e = Render(&out, tasks, format, true); e != nil || !strings.Contains(out.String(), *body) {
					t.Fatal(format, e)
				}
			}
		}
	}
}

type failingWriter struct {
	short  bool
	cancel context.CancelFunc
}

func (w failingWriter) Write(p []byte) (int, error) {
	if w.cancel != nil {
		w.cancel()
	}
	if w.short {
		return len(p) - 1, nil
	}
	return 0, errors.New("SECRET lower error")
}
func TestExitPriority(t *testing.T) {
	for _, counts := range []Counts{{}, {Failed: 1}, {Unreachable: 1}, {Cancelled: 1}} {
		for _, a := range []int{0, 1, 2, 3, 130} {
			for _, b := range []int{0, 1, 2, 3, 130} {
				want := max(a, b)
				if counts.Failed+counts.Unreachable > 0 {
					want = max(want, 1)
				}
				if counts.Cancelled > 0 {
					want = 130
				}
				if got := ExitCode(context.Background(), counts, a, b); got != want {
					t.Fatal(counts, a, b, got, want)
				}
			}
		}
	}
	if ExitCode(context.Background(), Counts{}, 99) != 3 {
		t.Fatal("invalid executor code")
	}
}
func TestFinishFailuresAndCancellation(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		for _, short := range []bool{false, true} {
			for _, cancelled := range []bool{false, true} {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				w := failingWriter{short: short}
				if cancelled {
					w.cancel = cancel
				}
				var diag bytes.Buffer
				want := 3
				if cancelled {
					want = 130
				}
				if got := Finish(ctx, w, &diag, fixture(), format, false, nil); got != 130 { // fixture itself includes cancelled
					t.Fatal(got, want)
				}
				tasks := []TaskResult{{Host: "a", Status: OK}}
				ctx2, cancel2 := context.WithCancel(context.Background())
				defer cancel2()
				if cancelled {
					w.cancel = cancel2
				}
				if got := Finish(ctx2, w, &diag, tasks, format, false, nil); got != want {
					t.Fatal(got, want)
				}
				if strings.Contains(diag.String(), "SECRET") {
					t.Fatal(diag.String())
				}
			}
		}
	}
	for _, failure := range []error{errors.New("SECRET"), &Error{Code: "SECRET"}, PublicError("input_error")} {
		var out, diag bytes.Buffer
		want := 3
		if failure.Error() == "input_error" {
			want = 2
		}
		if got := Finish(context.Background(), &out, &diag, nil, "json", false, failure); got != want || strings.Contains(out.String()+diag.String(), "SECRET") {
			t.Fatal(got, out.String(), diag.String())
		}
		if got := Finish(context.Background(), io.Discard, failingWriter{}, nil, "json", false, failure); got != 3 {
			t.Fatal(got)
		}
	}
}

func TestRenderBoundsExistingOutput(t *testing.T) {
	body := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{255}, OutputLimit+1))
	tasks := []TaskResult{{Host: "a", Status: OK, StdoutBase64: &body, StderrBase64: ptr("")}}
	r, e := Aggregate(tasks, true)
	if e != nil {
		t.Fatal(e)
	}
	task := r.Hosts[0].Tasks[0]
	decoded, e := base64.StdEncoding.DecodeString(*task.StdoutBase64)
	if e != nil || len(decoded) != OutputLimit || !task.StdoutTruncated || task.StderrTruncated {
		t.Fatal(task, e)
	}
	if *tasks[0].StdoutBase64 != body || tasks[0].StdoutTruncated {
		t.Fatal("mutated")
	}
	tasks[0].StdoutBase64 = ptr("invalid!")
	var out bytes.Buffer
	if _, e := Render(&out, tasks, "json", true); e == nil || out.Len() != 0 {
		t.Fatal(e)
	}
}

func TestClassifiedTaskFailures(t *testing.T) {
	for _, c := range []struct {
		code string
		want int
	}{{"deadline_exceeded", 1}, {"input_error", 2}, {"internal_error", 3}, {"SECRET", 3}, {"cancelled", 130}} {
		tasks := []TaskResult{{Host: "a", Status: Failed, Error: &Error{Code: c.code}}}
		if got := Finish(context.Background(), io.Discard, io.Discard, tasks, "json", false, nil); got != c.want {
			t.Fatal(c, got)
		}
	}
}
