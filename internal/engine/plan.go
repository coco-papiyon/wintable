package engine

import (
	"encoding/json"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/config"
	"github.com/coco-papiyon/wintable/internal/inventory"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/transport"
)

type TaskPlan struct {
	Name, Module string
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
	return build(b, limit, limitSet, module.WindowsBackends(), wrapperLines...)
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
		hp.Connection = transport.Host{Name: h, Address: connection["address"].(string), User: connection["user"].(string), PrivateKeyFile: connection["private_key_file"].(string), KnownHostsFile: connection["known_hosts_file"].(string), Port: uint16(port)}
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
			backend, ok := backends[module.Key{OS: hp.OS, Module: kind}]
			if !ok {
				return nil, fmt.Errorf("input_error: unsupported OS/module combination")
			}
			backend.Target = module.Key{OS: hp.OS, Module: kind}
			if e := backend.Check(); e != nil {
				return nil, e
			}
			payload, e := backend.Prepare(h, i+1, kind, args)
			if e != nil {
				return nil, e
			}
			hp.Tasks = append(hp.Tasks, TaskPlan{t["name"].(string), t["module"].(string), payload, &backend})
		}
		p.Hosts = append(p.Hosts, hp)
	}
	return p, nil
}
