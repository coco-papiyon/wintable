package module

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCRT(t *testing.T) {
	for _, c := range []struct{ in, want string }{{"", `""`}, {"abc", "abc"}, {"a b", `"a b"`}, {`a"b`, `"a\"b"`}, {"a b\\", `"a b\\"`}, {"a\\\"b", `"a\\\"b"`}} {
		if got := QuoteCRT(c.in); got != c.want {
			t.Errorf("%q: %q != %q", c.in, got, c.want)
		}
	}
	if UTF16Len("a😀") != 3 {
		t.Fatal("UTF16")
	}
	for _, n := range []int{32765, 32766, 32767} {
		exe := `C:\a.exe`
		arg := "😀" + strings.Repeat("x", n-len(exe)-5)
		_, e := Prepare("h", 1, "command", map[string]any{"executable": exe, "argv": []any{arg}})
		if (e == nil) != (n <= CommandLimit) {
			t.Fatalf("command %d: %v", n, e)
		}
	}
	for _, exe := range []string{`C:\CMD.EXE`, `C:\x.bat`, `C:\x.CMD`, `C:\x.ps1`, `C:\x.com`} {
		if _, e := Prepare("h", 1, "command", map[string]any{"executable": exe, "argv": []any{}}); e == nil {
			t.Fatal(exe)
		}
	}
}
func TestContentBoundary(t *testing.T) {
	for _, n := range []int{ContentLimit - 1, ContentLimit, ContentLimit + 1} {
		s := "日" + strings.Repeat("a", n-3)
		_, e := Prepare("h", 2, "file_content", map[string]any{"path": `C:\a`, "content": s})
		if (e == nil) != (n <= ContentLimit) {
			t.Fatalf("%d: %v", n, e)
		}
	}
}
func TestJSONBoundaryAndOwnership(t *testing.T) {
	// A path can be long independently of content and command limits. Newlines
	// in content exercise JSON escaping while remaining below the content limit.
	for _, n := range []int{JSONLimit - 1, JSONLimit, JSONLimit + 1} {
		args := map[string]any{"path": `C:\a`, "content": "\n日\""}
		seed, e := Prepare("h", 2, "file_content", args)
		if e != nil {
			t.Fatal(e)
		}
		args["path"] = args["path"].(string) + strings.Repeat("a", n-len(seed.JSON()))
		p, e := Prepare("h", 2, "file_content", args)
		if (e == nil) != (n <= JSONLimit) {
			t.Fatalf("%d: %v", n, e)
		}
		if e != nil {
			continue
		}
		original := p.JSON()
		if len(original) != n || !json.Valid(original) {
			t.Fatal("JSON size")
		}
		args["content"] = "changed"
		copy := p.JSON()
		copy[0] = '!'
		req, e := p.Request(`cmd.exe /c "fixed"`)
		if e != nil || !bytes.Equal(req.Stdin, original) {
			t.Fatal("mutated payload")
		}
		req.Stdin[0] = '!'
		if !bytes.Equal(p.JSON(), original) {
			t.Fatal("request aliases plan")
		}
	}
}
func TestWrapperBoundary(t *testing.T) {
	for _, n := range []int{7999, 8000, 8001} {
		e := ValidateWrapper("😀" + strings.Repeat("x", n-2))
		if (e == nil) != (n <= WrapperLimit) {
			t.Fatal(n, e)
		}
		if e != nil && !e.(*SizeError).Internal {
			t.Fatal("classification")
		}
	}
}
func TestSafeSizeDiagnostic(t *testing.T) {
	_, e := Prepare("host", 3, "file_content", map[string]any{"path": `C:\SECRET`, "content": strings.Repeat("SECRET", ContentLimit)})
	if e == nil || e.Error() != "host=\"host\" task=3 limit=1048576 actual=6291456" {
		t.Fatal(e)
	}
}
