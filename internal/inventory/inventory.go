package inventory

import (
	"fmt"
	"sort"
	"strings"
)

func invalid() error {
	return fmt.Errorf("input_error: inventory $: invalid selection or inventory membership")
}
func object(x any) map[string]any { m, _ := x.(map[string]any); return m }
func names(m map[string]any) []string {
	a := []string{}
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}

// Select validates the entire inventory, then returns a sorted unique intersection.
func Select(inv map[string]any, selector, limit string, limitSet bool) ([]string, error) {
	hosts, groups := object(inv["hosts"]), object(inv["groups"])
	for h := range hosts {
		if h == "all" || strings.TrimSpace(h) == "" {
			return nil, invalid()
		}
		if _, ok := groups[h]; ok {
			return nil, invalid()
		}
	}
	for g, x := range groups {
		if g == "all" || strings.TrimSpace(g) == "" {
			return nil, invalid()
		}
		for _, v := range object(x)["hosts"].([]any) {
			if _, ok := hosts[v.(string)]; !ok {
				return nil, invalid()
			}
		}
	}
	selected := map[string]bool{}
	if selector == "all" {
		for h := range hosts {
			selected[h] = true
		}
	} else if _, ok := hosts[selector]; ok {
		selected[selector] = true
	} else if g, ok := groups[selector]; ok {
		for _, h := range object(g)["hosts"].([]any) {
			selected[h.(string)] = true
		}
	} else {
		return nil, invalid()
	}
	if limitSet {
		allowed := map[string]bool{}
		for _, h := range strings.Split(limit, ",") {
			if _, ok := hosts[h]; !ok {
				return nil, invalid()
			}
			allowed[h] = true
		}
		for h := range selected {
			if !allowed[h] {
				delete(selected, h)
			}
		}
	}
	if len(selected) == 0 {
		return nil, invalid()
	}
	out := []string{}
	for h := range selected {
		out = append(out, h)
	}
	sort.Strings(out)
	return out, nil
}

// Variables returns a new dictionary including every group containing the host.
func Variables(inv map[string]any, host string, cli map[string]any) map[string]string {
	out := map[string]string{}
	merge := func(x any) {
		for k, v := range object(x) {
			out[k] = v.(string)
		}
	}
	merge(object(inv["defaults"])["vars"])
	groups := object(inv["groups"])
	for _, g := range names(groups) {
		m := object(groups[g])
		for _, h := range m["hosts"].([]any) {
			if h == host {
				merge(m["vars"])
				break
			}
		}
	}
	merge(object(object(inv["hosts"])[host])["vars"])
	merge(cli)
	return out
}

// Expand scans source once. Substituted values and escaped references are never rescanned.
func Expand(s string, vars map[string]string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "$${") {
			b.WriteString("${")
			i += 3
		} else if strings.HasPrefix(s[i:], "${") {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				return "", fmt.Errorf("input_error: tasks $: malformed variable reference")
			}
			key := s[i+2 : i+2+end]
			v, ok := vars[key]
			if !ok {
				return "", fmt.Errorf("input_error: tasks $: undefined variable")
			}
			b.WriteString(v)
			i += end + 3
		} else {
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), nil
}
