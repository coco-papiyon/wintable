package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/module"
)

func TestScriptPreflightNoConnections(t *testing.T) {
	for _, kind := range []string{"powershell", "shell"} {
		for _, failure := range []string{"OS", "undefined", "size", "json", "delegate"} {
			validFiles(t)
			raw, e := os.ReadFile("i")
			if e != nil {
				t.Fatal(e)
			}
			var inv map[string]any
			if e = json.Unmarshal(raw, &inv); e != nil {
				t.Fatal(e)
			}
			hosts := inv["hosts"].(map[string]any)
			var last map[string]any
			for _, v := range hosts {
				h := v.(map[string]any)
				h["os"] = "windows"
				if kind == "shell" {
					h["os"] = "linux"
				}
				h["vars"] = map[string]any{"code": "exit 0"}
				last = map[string]any{}
				for k, x := range h {
					last[k] = x
				}
			}
			vars := map[string]any{"code": "exit 0"}
			last["vars"] = vars
			hosts["zz-last"] = last
			switch failure {
			case "OS":
				if kind == "shell" {
					last["os"] = "windows"
				} else {
					last["os"] = "linux"
				}
			case "undefined":
				delete(vars, "code")
			case "size":
				vars["code"] = strings.Repeat("x", module.ContentLimit+1)
			case "json":
				vars["code"] = "x" + strings.Repeat("\n", module.ContentLimit-1)
			}
			task := map[string]any{"name": "script", "module": kind, "args": map[string]any{"script": "${code}"}}
			if failure == "delegate" {
				task["delegate_to"] = "localhost"
			}
			raw, _ = json.Marshal(inv)
			if e = os.WriteFile("i", raw, 0600); e != nil {
				t.Fatal(e)
			}
			raw, _ = json.Marshal(map[string]any{"version": 1, "hosts": "all", "tasks": []any{task}})
			if e = os.WriteFile("t", raw, 0600); e != nil {
				t.Fatal(e)
			}
			f := &fakeConnector{}
			var out, diag bytes.Buffer
			code := RunEngine(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t"}, &out, &diag, engine.Runtime{Connector: f})
			if code != 2 || f.calls != 0 {
				t.Fatal(kind, failure, code, f.calls, diag.String())
			}
		}
	}
}
