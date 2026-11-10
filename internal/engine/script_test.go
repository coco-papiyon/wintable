package engine

import (
	"github.com/coco-papiyon/wintable/internal/module"
	"strings"
	"testing"
)

func TestScriptPlan(t *testing.T) {
	for _, tc := range []struct{ kind, os string }{{"powershell", "windows"}, {"shell", "linux"}} {
		for _, failure := range []string{"", "OS", "undefined", "size", "json"} {
			b := bundle()
			b.Tasks["tasks"] = []any{map[string]any{"name": "script", "module": tc.kind, "args": map[string]any{"script": "echo ${v}"}}}
			hosts := b.Inventory["hosts"].(map[string]any)
			for _, v := range hosts {
				v.(map[string]any)["os"] = tc.os
			}
			last := hosts["z"].(map[string]any)
			vars := last["vars"].(map[string]any)
			switch failure {
			case "OS":
				if tc.os == "windows" {
					last["os"] = "linux"
				} else {
					last["os"] = "windows"
				}
			case "undefined":
				delete(vars, "v")
			case "size":
				vars["v"] = strings.Repeat("x", module.ContentLimit+1)
			case "json":
				vars["v"] = strings.Repeat("\n", module.ContentLimit-10)
			}
			p, e := Build(b, "", false)
			if failure != "" {
				if e == nil || p != nil {
					t.Fatal(tc, failure, "partial plan")
				}
				continue
			}
			if e != nil {
				t.Fatal(e)
			}
			if !strings.Contains(string(p.Hosts[0].Tasks[0].Payload.JSON()), "echo ok") {
				t.Fatal("variable not expanded")
			}
		}
	}
}
