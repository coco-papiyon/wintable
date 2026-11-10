package module

import "testing"

func TestFilePrepare(t *testing.T) {
	for _, os := range []string{"windows", "linux"} {
		b := FileBackend(os)
		if e := b.Check(); e != nil {
			t.Fatal(os, e)
		}
		good := "/var/lib/日本語 space/a&b"
		if os == "windows" {
			good = `C:\日本語 space\a&b`
		}
		if _, e := b.Prepare("h", 1, "file", map[string]any{"path": good, "state": "directory"}); e != nil {
			t.Fatal(e)
		}
		paths := []string{"", "relative", "/", "/a/../b", "/a/", "/a//b", "/a\x00b"}
		if os == "windows" {
			paths = []string{`C:\`, `C:\a\..\b`, `C:\a\`, `C:\a\\b`, `C:\NUL.txt`, `C:\COM¹`, `C:\a.`, `C:\a `, `C:\a:b`, `\\host\share`}
		}
		for _, p := range paths {
			if _, e := b.Prepare("h", 1, "file", map[string]any{"path": p, "state": "directory"}); e == nil {
				t.Fatal("accepted", os, p)
			}
		}
		for _, a := range []map[string]any{{"path": good, "state": "absent"}, {"path": good}, {"path": good, "state": "directory", "extra": "x"}, {"path": 1, "state": "directory"}} {
			if _, e := b.Prepare("h", 1, "file", a); e == nil {
				t.Fatal(a)
			}
		}
	}
}
