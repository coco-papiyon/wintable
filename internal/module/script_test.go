package module

import (
	"strings"
	"testing"
)

func TestScriptPreparation(t *testing.T) {
	for _, kind := range []string{"shell", "powershell"} {
		for _, s := range []string{"", " \r\n", "x\x00", "\xff", strings.Repeat("x", ContentLimit+1)} {
			if _, e := PrepareScript("h", 1, kind, map[string]any{"script": s}); e == nil {
				t.Fatal("accepted invalid script")
			}
		}
		for _, n := range []int{ContentLimit - 1, ContentLimit} {
			if _, e := PrepareScript("h", 1, kind, map[string]any{"script": strings.Repeat("#", n)}); e != nil {
				t.Fatal(e)
			}
		}
		if _, e := PrepareScript("h", 1, kind, map[string]any{"script": strings.Repeat("\n", ContentLimit-1) + "x"}); e == nil {
			t.Fatal("JSON expansion limit")
		}
		for _, a := range []map[string]any{{}, {"script": 1}, {"script": "exit 0", "extra": "x"}} {
			if _, e := PrepareScript("h", 1, kind, a); e == nil {
				t.Fatal("shape")
			}
		}
	}
	b := DefaultBackends()
	for _, k := range []Key{{"windows", "powershell"}, {"linux", "shell"}} {
		if e := b[k].Check(); e != nil {
			t.Fatal(k, e)
		}
	}
	for _, k := range []Key{{"linux", "powershell"}, {"windows", "shell"}} {
		if _, ok := b[k]; ok {
			t.Fatal("cross OS fallback")
		}
	}
}
