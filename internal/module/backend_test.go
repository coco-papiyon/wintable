package module

import (
	"strings"
	"testing"
)

func TestLinuxBoundaries(t *testing.T) {
	for _, n := range []int{LinuxArgumentLimit - 1, LinuxArgumentLimit, LinuxArgumentLimit + 1} {
		_, e := PrepareLinuxCommand("h", 1, "command", map[string]any{"executable": "/bin/x", "argv": []any{strings.Repeat("x", n)}})
		if (e == nil) != (n <= LinuxArgumentLimit) {
			t.Fatal(n, e)
		}
	}
	for _, n := range []int{LinuxArgumentsLimit - 1, LinuxArgumentsLimit, LinuxArgumentsLimit + 1} {
		// /x plus NUL, two argument terminators.
		a := LinuxArgumentLimit
		b := n - 3 - 2 - a
		_, e := PrepareLinuxCommand("h", 1, "command", map[string]any{"executable": "/x", "argv": []any{strings.Repeat("x", a), strings.Repeat("x", b)}})
		if (e == nil) != (n <= LinuxArgumentsLimit) {
			t.Fatal(n, e)
		}
	}
	for _, n := range []int{4095, 4096, 4097} {
		a := make([]any, n)
		for i := range a {
			a[i] = ""
		}
		_, e := PrepareLinuxCommand("h", 1, "command", map[string]any{"executable": "/x", "argv": a})
		if (e == nil) != (n <= 4096) {
			t.Fatal(n, e)
		}
	}
	for _, n := range []int{8191, 8192, 8193} {
		if (ValidateLinuxWrapper(strings.Repeat("x", n)) == nil) != (n <= 8192) {
			t.Fatal(n)
		}
	}
	for _, s := range []string{"", "relative", "/", "/x/", "/./x", "/x/../y", "/x\n", "/x\x00", "/\xff"} {
		if _, e := PrepareLinuxCommand("h", 1, "command", map[string]any{"executable": s, "argv": []any{}}); e == nil {
			t.Fatal(s)
		}
	}
	if _, e := PrepareLinuxCommand("h", 1, "command", map[string]any{"executable": "/日本語", "argv": []any{"", "😀\n' &"}}); e != nil {
		t.Fatal(e)
	}
	if ValidateLinuxWrapper(strings.Repeat("😀", 2048)) != nil || ValidateLinuxWrapper(strings.Repeat("😀", 2049)) == nil {
		t.Fatal("UTF-8 limit")
	}
	if _, ok := WindowsBackends()[Key{"linux", "command"}]; ok {
		t.Fatal("Linux registered")
	}
}
