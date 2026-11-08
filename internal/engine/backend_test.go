package engine

import (
	"context"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"testing"
	"time"
)

func TestMixedBackends(t *testing.T) {
	b := bundle()
	hosts := b.Inventory["hosts"].(map[string]any)
	for name, v := range hosts {
		h := v.(map[string]any)
		os, exe := "windows", `C:\x.exe`
		if name == "z" {
			os, exe = "linux", "/bin/x"
		}
		h["os"] = os
		h["vars"] = map[string]any{"exe": exe}
	}
	b.Tasks["tasks"] = []any{map[string]any{"name": "run", "module": "command", "args": map[string]any{"executable": "${exe}", "argv": []any{}}}}
	registry := module.WindowsBackends()
	decoded := 0
	registry[module.Key{OS: "linux", Module: "command"}] = module.Backend{Prepare: module.PrepareLinuxCommand, Command: "linux-fixed", LaunchLine: "linux-fixed", ValidateLaunch: module.ValidateLinuxWrapper, Decode: func(transport.Response, bool) result.TaskResult {
		decoded++
		return result.TaskResult{Status: result.OK}
	}}
	p, e := BuildWithBackends(b, "", false, registry)
	if e != nil {
		t.Fatal(e)
	}
	runs, connects := 0, 0
	r := Runtime{Connector: connectorFunc(func(_ context.Context, h transport.Host) (transport.Session, error) {
		connects++
		return &session{run: func(_ context.Context, q transport.Request) (transport.Response, error) {
			runs++
			if h.Name == "z" && q.Command != "linux-fixed" {
				t.Error(q.Command)
			}
			code := 0
			return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code, Stdout: []byte(`{"exit_code":0,"stdout":"","stderr":"","stdout_truncated":false,"stderr_truncated":false}`)}, nil
		}, close: func() error { return nil }}, nil
	})}
	rows, e := r.Execute(context.Background(), p, 1, time.Second, false)
	if e != nil || len(rows) != 2 || decoded != 1 || runs != 2 {
		t.Fatal(rows, e, decoded, runs)
	}
	p.Hosts[1].OS = "windows"
	connects = 0
	if _, e = r.Execute(context.Background(), p, 1, time.Second, false); e == nil || connects != 0 {
		t.Fatal("mismatched backend connected")
	}
	if p, e = Build(b, "", false); e != nil || p.Hosts[1].Tasks[0].Backend.Command != module.LinuxCommandWrapper() {
		t.Fatal("Linux production backend", e)
	}
	hosts["z"].(map[string]any)["vars"] = map[string]any{"exe": "relative"}
	if p, e = BuildWithBackends(b, "", false, registry); p != nil || e == nil {
		t.Fatal("partial invalid plan")
	}
}
func TestWindowsOSDefaultEquivalence(t *testing.T) {
	b := bundle()
	a, e := Build(b, "", false)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range b.Inventory["hosts"].(map[string]any) {
		v.(map[string]any)["os"] = "windows"
	}
	c, e := Build(b, "", false)
	if e != nil {
		t.Fatal(e)
	}
	for i, h := range a.Hosts {
		for j, x := range h.Tasks {
			y := c.Hosts[i].Tasks[j]
			if string(x.Payload.JSON()) != string(y.Payload.JSON()) || x.Backend.Command != y.Backend.Command {
				t.Fatal("default changed")
			}
		}
	}
}
