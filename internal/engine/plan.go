package engine

import (
	"encoding/json"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/config"
	"github.com/coco-papiyon/wintable/internal/inventory"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/transport"
	"runtime"
	"strings"
)

type TaskPlan struct {
	Name, Module string
	Local        bool
	Payload      module.Prepared
	Backend      *module.Backend
}
type HostPlan struct {
	OS         string
	Name       string
	Connection transport.Host
	Tasks      []TaskPlan
}
type Plan struct{ Hosts []HostPlan }

// Build has no connector capability. On any failure no partial plan is returned.
// wrapperLines are final launch lines from the distribution provider, checked
// before the caller may connect. Real wrappers are provided in I-07/I-08.
func Build(b *config.Bundle, limit string, limitSet bool, wrapperLines ...string) (*Plan, error) {
	return build(b, limit, limitSet, module.DefaultBackends(), wrapperLines...)
}
func BuildWithBackends(b *config.Bundle, limit string, limitSet bool, backends module.Backends) (*Plan, error) {
	return build(b, limit, limitSet, backends)
}
func build(b *config.Bundle, limit string, limitSet bool, backends module.Backends, wrapperLines ...string) (*Plan, error) {
	for _, line := range wrapperLines {
		if e := module.ValidateWrapper(line); e != nil {
			return nil, e
		}
	}
	hosts, e := inventory.Select(b.Inventory, b.Tasks["hosts"].(string), limit, limitSet)
	if e != nil {
		return nil, e
	}
	p := &Plan{}
	fetchPaths := []string{}
	for _, h := range hosts {
		hp := HostPlan{Name: h}
		connection := b.Inventory["hosts"].(map[string]any)[h].(map[string]any)
		hp.OS, _ = connection["os"].(string)
		if hp.OS == "" {
			hp.OS = "windows"
		}
		if hp.OS != "windows" && hp.OS != "linux" {
			return nil, fmt.Errorf("input_error: invalid target OS")
		}
		port, _ := connection["port"].(json.Number).Int64()
		str := func(k string) string { v, _ := connection[k].(string); return v }
		hp.Connection = transport.Host{Name: h, Address: str("address"), User: str("user"), PrivateKeyFile: str("private_key_file"), KnownHostsFile: str("known_hosts_file"), Port: uint16(port)}
		vars := inventory.Variables(b.Inventory, h, b.Vars)
		for i, x := range b.Tasks["tasks"].([]any) {
			t := x.(map[string]any)
			args := map[string]any{}
			for k, v := range t["args"].(map[string]any) {
				switch v := v.(type) {
				case string:
					args[k], e = inventory.Expand(v, vars)
				case []any:
					a := []any{}
					for _, s := range v {
						var expanded string
						expanded, e = inventory.Expand(s.(string), vars)
						if e != nil {
							break
						}
						a = append(a, expanded)
					}
					args[k] = a
				}
				if e != nil {
					return nil, e
				}
			}
			kind := t["module"].(string)
			local := t["delegate_to"] == "localhost"
			targetOS := hp.OS
			if local {
				if runtime.GOOS != "windows" || kind != "command" {
					return nil, fmt.Errorf("input_error: localhost command requires Windows controller")
				}
				targetOS = "windows"
			}
			backend, ok := backends[module.Key{OS: targetOS, Module: kind}]
			if !ok {
				return nil, fmt.Errorf("input_error: unsupported OS/module combination")
			}
			backend.Target = module.Key{OS: targetOS, Module: kind}
			if e := backend.Check(); e != nil {
				return nil, e
			}
			payload, e := backend.Prepare(h, i+1, kind, args)
			if e != nil {
				return nil, e
			}
			if kind == "fetch" {
				path := strings.ToLower(payload.FetchDestination)
				for _, other := range fetchPaths {
					if path == other || strings.HasPrefix(path, other+`\`) || strings.HasPrefix(other, path+`\`) {
						return nil, fmt.Errorf("input_error: conflicting fetch destinations")
					}
				}
				fetchPaths = append(fetchPaths, path)
			}
			hp.Tasks = append(hp.Tasks, TaskPlan{Name: t["name"].(string), Module: kind, Local: local, Payload: payload, Backend: &backend})
		}
		p.Hosts = append(p.Hosts, hp)
	}
	return p, nil
}
