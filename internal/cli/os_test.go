package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/engine"
	"os"
	"testing"
)

func TestOSPreflightNoConnections(t *testing.T) {
	for _, target := range []string{"linux", "unknown", "windows"} {
		for _, kind := range []string{"command", "file_content"} {
			validFiles(t)
			raw, e := os.ReadFile("i")
			if e != nil {
				t.Fatal(e)
			}
			var inv map[string]any
			json.Unmarshal(raw, &inv)
			hosts := inv["hosts"].(map[string]any)
			for _, v := range hosts {
				h := v.(map[string]any)
				clone := map[string]any{}
				for k, x := range h {
					clone[k] = x
				}
				clone["os"] = target
				hosts["zz-last"] = clone
				break
			}
			raw, _ = json.Marshal(inv)
			os.WriteFile("i", raw, 0600)
			args := map[string]any{"executable": "relative", "argv": []any{}}
			if kind == "file_content" {
				args = map[string]any{"path": "relative", "content": ""}
			}
			raw, _ = json.Marshal(map[string]any{"version": 1, "hosts": "all", "tasks": []any{map[string]any{"name": "bad", "module": kind, "args": args}}})
			os.WriteFile("t", raw, 0600)
			f := &fakeConnector{}
			var out, diag bytes.Buffer
			if code := RunEngine(context.Background(), []string{"run", "--inventory", "i", "--tasks", "t"}, &out, &diag, engine.Runtime{Connector: f}); code != 2 || f.calls != 0 {
				t.Fatal(target, kind, code, f.calls, diag.String())
			}
		}
	}
}
