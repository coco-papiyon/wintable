package inventory

import (
	"reflect"
	"testing"
)

func fixture() map[string]any {
	return map[string]any{"defaults": map[string]any{"vars": map[string]any{"x": "default"}}, "hosts": map[string]any{"a": map[string]any{"vars": map[string]any{"x": "host"}}, "b": map[string]any{}}, "groups": map[string]any{"z": map[string]any{"hosts": []any{"a", "b", "a"}, "vars": map[string]any{"x": "z", "y": "z"}}, "g": map[string]any{"hosts": []any{"a"}, "vars": map[string]any{"x": "g", "y": "g"}}}}
}
func TestSelect(t *testing.T) {
	for _, c := range []struct {
		s, l string
		set  bool
		want []string
	}{{"all", "", false, []string{"a", "b"}}, {"z", "a,a", true, []string{"a"}}, {"a", "", false, []string{"a"}}, {"g", "b", true, nil}, {"all", "", true, nil}, {"unknown", "", false, nil}, {"all", "g", true, nil}, {"all", "a,", true, nil}, {"ALL", "", false, nil}} {
		got, e := Select(fixture(), c.s, c.l, c.set)
		if !reflect.DeepEqual(got, c.want) || (e == nil) != (c.want != nil) {
			t.Fatal(c, got, e)
		}
	}
	for _, kind := range []string{"collision", "reservedHost", "reservedGroup", "member", "empty"} {
		m := fixture()
		h := object(m["hosts"])
		g := object(m["groups"])
		switch kind {
		case "collision":
			g["a"] = g["z"]
		case "reservedHost":
			h["all"] = h["a"]
		case "reservedGroup":
			g["all"] = g["z"]
		case "member":
			object(g["g"])["hosts"] = []any{"missing"}
		case "empty":
			g["empty"] = map[string]any{"hosts": []any{}}
		}
		selector := "a"
		if kind == "empty" {
			selector = "empty"
		}
		if _, e := Select(m, selector, "", false); e == nil {
			t.Fatal(kind)
		}
	}
}
func TestVariables(t *testing.T) {
	m := fixture()
	a := Variables(m, "a", nil)
	b := Variables(m, "b", nil)
	if a["x"] != "host" || a["y"] != "z" || b["x"] != "z" {
		t.Fatal(a, b)
	}
	a["x"] = "mutated"
	if Variables(m, "a", map[string]any{"x": "cli"})["x"] != "cli" || b["x"] != "z" {
		t.Fatal("shared")
	}
}
func TestExpand(t *testing.T) {
	vars := map[string]string{"x": "${y}", "y": "日本語"}
	for _, c := range []struct{ in, want string }{{"${x}${y}", "${y}日本語"}, {"$${y}", "${y}"}, {"$${missing}${y}", "${missing}日本語"}} {
		got, e := Expand(c.in, vars)
		if e != nil || got != c.want {
			t.Fatal(got, e)
		}
	}
	for _, s := range []string{"${missing}", "${y", "${}"} {
		if _, e := Expand(s, vars); e == nil {
			t.Fatal(s)
		}
	}
}
