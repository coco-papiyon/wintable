package engine

import (
	"bytes"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/config"
	"github.com/coco-papiyon/wintable/internal/module"
	"strings"
	"testing"
)

func bundle() *config.Bundle {
	hosts := map[string]any{}
	for _, h := range []string{"a", "z"} {
		hosts[h] = map[string]any{"address": "localhost", "port": json.Number("22"), "user": "u", "private_key_file": "k", "known_hosts_file": "kh", "vars": map[string]any{"v": "ok"}}
	}
	return &config.Bundle{Inventory: map[string]any{"hosts": hosts}, Tasks: map[string]any{"hosts": "all", "tasks": []any{map[string]any{"name": "${literal}", "module": "file_content", "args": map[string]any{"path": `C:\a`, "content": "first"}}, map[string]any{"name": "last", "module": "file_content", "args": map[string]any{"path": `C:\a`, "content": "${v}"}}}}}
}
func TestNoPartialPlan(t *testing.T) {
	for _, kind := range []string{"content", "json", "command", "path", "undefined"} {
		b := bundle()
		last := b.Tasks["tasks"].([]any)[1].(map[string]any)
		vars := b.Inventory["hosts"].(map[string]any)["z"].(map[string]any)["vars"].(map[string]any)
		switch kind {
		case "content":
			vars["v"] = strings.Repeat("s", module.ContentLimit+1)
		case "json":
			vars["v"] = strings.Repeat("a", module.JSONLimit)
			last["args"] = map[string]any{"path": `C:\${v}`, "content": ""}
		case "command":
			vars["v"] = strings.Repeat("a", module.CommandLimit)
			last["module"] = "command"
			last["args"] = map[string]any{"executable": `C:\a.exe`, "argv": []any{"${v}"}}
		case "path":
			vars["v"] = "relative"
			last["args"] = map[string]any{"path": "${v}", "content": ""}
			b.Inventory["hosts"].(map[string]any)["a"].(map[string]any)["vars"] = map[string]any{"v": `C:\ok`}
		case "undefined":
			delete(vars, "v")
		}
		p, e := Build(b, "", false)
		if p != nil || e == nil {
			t.Fatal(kind, p, e)
		}
	}
}
func TestPlanSnapshot(t *testing.T) {
	b := bundle()
	p, e := Build(b, "", false, `cmd.exe /c "fixed"`)
	if e != nil {
		t.Fatal(e)
	}
	if p.Hosts[0].Name != "a" || p.Hosts[1].Name != "z" || p.Hosts[0].Tasks[0].Name != "${literal}" {
		t.Fatal(p)
	}
	original := p.Hosts[0].Tasks[1].Payload.JSON()
	b.Tasks["tasks"].([]any)[1].(map[string]any)["args"].(map[string]any)["content"] = "mutated"
	req, e := p.Hosts[0].Tasks[1].Payload.Request(`cmd.exe /c "fixed"`)
	if e != nil || !bytes.Equal(req.Stdin, original) {
		t.Fatal("changed serialized input", e)
	}
	if p.Hosts[0].Connection.Port != 22 {
		t.Fatal("connection snapshot")
	}
}
