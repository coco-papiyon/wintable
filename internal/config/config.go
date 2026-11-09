package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bundle contains schema-validated inputs; selection and expansion are separate.
type Bundle struct {
	Inventory, Tasks map[string]any
	Vars             map[string]any
}
type Error struct{ Kind, Path, Reason string }

func (e *Error) Error() string {
	return fmt.Sprintf("input_error: %s %s: %s", e.Kind, e.Path, e.Reason)
}
func bad(k, p, r string) error { return &Error{k, p, r} }
func read(k, path string) (map[string]any, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, bad(k, "$", "cannot read file")
	}
	b = bytes.TrimPrefix(b, []byte{239, 187, 191})
	if !utf8.Valid(b) {
		return nil, bad(k, "$", "invalid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	x, e := value(d, k, "$")
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, bad(k, "$", "trailing data")
	}
	m, ok := x.(map[string]any)
	if !ok {
		return nil, bad(k, "$", "expected object")
	}
	return m, nil
}
func value(d *json.Decoder, k, p string) (any, error) {
	t, e := d.Token()
	if e != nil {
		return nil, bad(k, p, "invalid JSON")
	}
	if c, ok := t.(json.Delim); ok {
		switch c {
		case '{':
			m := map[string]any{}
			i := 0
			for d.More() {
				t, e := d.Token()
				if e != nil {
					return nil, bad(k, p, "invalid JSON")
				}
				key, ok := t.(string)
				if !ok {
					return nil, bad(k, p, "invalid JSON")
				}
				loc := fmt.Sprintf("%s{member:%d}", p, i)
				i++
				if _, ok = m[key]; ok {
					return nil, bad(k, loc, "duplicate key")
				}
				x, e := value(d, k, loc)
				if e != nil {
					return nil, e
				}
				m[key] = x
			}
			if _, e := d.Token(); e != nil {
				return nil, bad(k, p, "invalid JSON")
			}
			return m, nil
		case '[':
			a := []any{}
			for d.More() {
				x, e := value(d, k, fmt.Sprintf("%s[%d]", p, len(a)))
				if e != nil {
					return nil, e
				}
				a = append(a, x)
			}
			if _, e := d.Token(); e != nil {
				return nil, bad(k, p, "invalid JSON")
			}
			return a, nil
		}
		return nil, bad(k, p, "invalid JSON")
	}
	return t, nil
}

type validator struct {
	k string
	e error
}

func (v *validator) fail(p, r string) {
	if v.e == nil {
		v.e = bad(v.k, p, r)
	}
}
func keys(m map[string]any) []string {
	a := []string{}
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func (v *validator) obj(x any, p string, allowed ...string) map[string]any {
	m, ok := x.(map[string]any)
	if !ok {
		v.fail(p, "expected object")
		return map[string]any{}
	}
	for _, k := range keys(m) {
		found := false
		for _, a := range allowed {
			if k == a {
				found = true
			}
		}
		if !found {
			v.fail(p, "unknown field")
		}
	}
	return m
}
func (v *validator) str(x any, p string, empty bool) string {
	s, ok := x.(string)
	if !ok || (!empty && strings.TrimSpace(s) == "") {
		v.fail(p, "expected string (nonempty where required)")
	}
	return s
}
func (v *validator) array(x any, p string) []any {
	a, ok := x.([]any)
	if !ok {
		v.fail(p, "expected array")
	}
	return a
}
func (v *validator) strings(x any, p string, empty bool) {
	for i, x := range v.array(x, p) {
		v.str(x, fmt.Sprintf("%s[%d]", p, i), empty)
	}
}

var variable = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")

func (v *validator) vars(x any, p string) {
	m, ok := x.(map[string]any)
	if !ok {
		v.fail(p, "expected string dictionary")
	}
	for i, k := range keys(m) {
		loc := fmt.Sprintf("%s{member:%d}", p, i)
		if !variable.MatchString(k) {
			v.fail(loc, "invalid variable name")
		}
		v.str(m[k], loc, true)
	}
}
func (v *validator) optionalVars(m map[string]any, p string) {
	if x, ok := m["vars"]; ok {
		v.vars(x, p+".vars")
	}
}
func (v *validator) version(m map[string]any) {
	if m["version"] != json.Number("1") {
		v.fail("$.version", "expected integer 1")
	}
}

// WindowsAbsolute validates target paths without control-side filepath conversion.
// Initial v1 accepts drive-rooted paths and excludes UNC/device namespaces.
func WindowsAbsolute(s string) bool {
	return len(s) >= 3 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':' && s[2] == '\\' && !strings.ContainsAny(s[3:], "<>:\"|?*/") && !strings.ContainsFunc(s, unicode.IsControl)
}
func LoadInventory(path string) (map[string]any, error) {
	m, e := read("inventory", path)
	if e != nil {
		return nil, e
	}
	v := validator{k: "inventory"}
	v.obj(m, "$", "version", "defaults", "groups", "hosts")
	v.version(m)
	if x, ok := m["defaults"]; ok {
		d := v.obj(x, "$.defaults", "vars")
		v.optionalVars(d, "$.defaults")
	}
	if x, ok := m["groups"]; ok {
		groups, ok := x.(map[string]any)
		if !ok {
			v.fail("$.groups", "expected object")
		}
		for i, k := range keys(groups) {
			p := fmt.Sprintf("$.groups{member:%d}", i)
			g := v.obj(groups[k], p, "hosts", "vars")
			v.strings(g["hosts"], p+".hosts", false)
			v.optionalVars(g, p)
		}
	}
	hosts, ok := m["hosts"].(map[string]any)
	if !ok || len(hosts) == 0 {
		v.fail("$.hosts", "expected nonempty object")
	}
	base, e := filepath.Abs(filepath.Dir(path))
	if e != nil {
		return nil, bad("inventory", "$", "cannot resolve path")
	}
	for i, k := range keys(hosts) {
		p := fmt.Sprintf("$.hosts{member:%d}", i)
		h := v.obj(hosts[k], p, "address", "port", "user", "private_key_file", "known_hosts_file", "vars", "os")
		v.optionalVars(h, p)
		if x, ok := h["os"]; ok {
			target := v.str(x, p+".os", false)
			if target != "windows" && target != "linux" {
				v.fail(p+".os", "expected windows or linux")
			}
		} else {
			h["os"] = "windows"
		}
		for _, f := range []string{"address", "user", "private_key_file", "known_hosts_file"} {
			s := v.str(h[f], p+"."+f, false)
			if strings.ContainsFunc(s, unicode.IsControl) || strings.ContainsAny(s, "${}") {
				v.fail(p+"."+f, "invalid connection value")
			}
			if f == "address" && strings.ContainsAny(s, " /\\") {
				v.fail(p+".address", "invalid address")
			}
			if f == "private_key_file" || f == "known_hosts_file" {
				if !filepath.IsAbs(s) {
					h[f] = filepath.Join(base, s)
				}
			}
		}
		if x, ok := h["port"]; ok {
			n, ok := x.(json.Number)
			if !ok {
				v.fail(p+".port", "expected port 1..65535")
			} else {
				n, e := n.Int64()
				if e != nil || n < 1 || n > 65535 {
					v.fail(p+".port", "expected port 1..65535")
				}
			}
		} else {
			h["port"] = json.Number("22")
		}
	}
	return m, v.e
}
func LoadTasks(path string) (map[string]any, error) {
	m, e := read("tasks", path)
	if e != nil {
		return nil, e
	}
	v := validator{k: "tasks"}
	v.obj(m, "$", "version", "hosts", "tasks")
	v.version(m)
	v.str(m["hosts"], "$.hosts", false)
	a := v.array(m["tasks"], "$.tasks")
	if len(a) == 0 {
		v.fail("$.tasks", "expected nonempty array")
	}
	for i, x := range a {
		p := fmt.Sprintf("$.tasks[%d]", i)
		t := v.obj(x, p, "name", "module", "args")
		v.str(t["name"], p+".name", false)
		module := v.str(t["module"], p+".module", false)
		switch module {
		case "command":
			g := v.obj(t["args"], p+".args", "executable", "argv")
			v.str(g["executable"], p+".args.executable", false)
			v.strings(g["argv"], p+".args.argv", true)
		case "file_content":
			g := v.obj(t["args"], p+".args", "path", "content")
			v.str(g["path"], p+".args.path", false)
			v.str(g["content"], p+".args.content", true)
		case "fetch":
			g := v.obj(t["args"], p+".args", "src", "dest")
			v.str(g["src"], p+".args.src", false)
			v.str(g["dest"], p+".args.dest", false)
		default:
			v.fail(p+".module", "unknown module")
		}
	}
	return m, v.e
}
func LoadVars(path string) (map[string]any, error) {
	m, e := read("vars-file", path)
	if e != nil {
		return nil, e
	}
	v := validator{k: "vars-file"}
	v.vars(m, "$")
	return m, v.e
}
func Load(inventory, tasks, vars string) (*Bundle, error) {
	b := &Bundle{Vars: map[string]any{}}
	var e error
	b.Inventory, e = LoadInventory(inventory)
	if e != nil {
		return nil, e
	}
	b.Tasks, e = LoadTasks(tasks)
	if e != nil {
		return nil, e
	}
	if vars != "" {
		b.Vars, e = LoadVars(vars)
		if e != nil {
			return nil, e
		}
	}
	return b, nil
}
