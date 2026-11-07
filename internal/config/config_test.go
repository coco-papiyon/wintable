package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const inventory = `{"version":1,"defaults":{"vars":{"message":"こんにちは"}},"groups":{"windows":{"hosts":["win"],"vars":{}}},"hosts":{"win":{"address":"win.example.test","port":22,"user":"automation","private_key_file":"keys/秘密 key","known_hosts_file":"known hosts","vars":{}}}}`
const tasks = `{"version":1,"hosts":"windows","tasks":[{"name":"接続確認","module":"command","args":{"executable":"C:\\Windows\\System32\\whoami.exe","argv":["", "日本語"]}},{"name":"配置","module":"file_content","args":{"path":"C:\\Temp\\日本語 file.txt","content":"${message}\n"}}]}`

func write(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "日本語 file.json")
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestValid(t *testing.T) {
	for _, prefix := range []string{"", "\xef\xbb\xbf"} {
		p := write(t, []byte(prefix+strings.ReplaceAll(inventory, ",", ",\r\n")))
		m, e := LoadInventory(p)
		if e != nil {
			t.Fatal(e)
		}
		h := m["hosts"].(map[string]any)["win"].(map[string]any)
		if h["private_key_file"] != filepath.Join(filepath.Dir(p), "keys/秘密 key") {
			t.Fatal(h["private_key_file"])
		}
		got, e := LoadTasks(write(t, []byte(prefix+tasks)))
		if e != nil {
			t.Fatal(e)
		}
		s := got["tasks"].([]any)[1].(map[string]any)["args"].(map[string]any)["path"]
		if s != "C:\\Temp\\日本語 file.txt" {
			t.Fatal(s)
		}
		if _, e := LoadVars(write(t, []byte(prefix+`{"_ok1":"日本語","Empty":""}`))); e != nil {
			t.Fatal(e)
		}
	}
}
func TestStrictJSON(t *testing.T) {
	for _, body := range []string{"", " ", "---\nversion: 1", "[]", "null", "{} {}", "{}x", "{", "{\"x\":1,}", "{/*comment*/}", "{\"x\":1,\"x\":2}", "{\"x\":{\"a\":1,\"a\":2}}", "{\"x\":[{\"a\":1,\"a\":2}]}", "\xef\xbb\xbf\xef\xbb\xbf{}", "{\"x\":\"\xff\"}"} {
		t.Run(body, func(t *testing.T) {
			p := write(t, []byte(body))
			for _, kind := range []string{"inventory", "tasks", "vars-file"} {
				if _, e := read(kind, p); e == nil {
					t.Fatalf("accepted %q", body)
				}
			}
		})
	}
}
func TestSchemas(t *testing.T) {
	cases := []struct{ kind, body string }{
		{"inventory", strings.Replace(inventory, `"version":1`, `"version":1,"extra":true`, 1)},
		{"inventory", strings.Replace(inventory, `"version":1,`, ``, 1)},
		{"inventory", strings.Replace(inventory, `"vars":{"message":"こんにちは"}`, `"vars":{"1bad":"secret"}`, 1)},
		{"inventory", strings.Replace(inventory, `"port":22`, `"port":null`, 1)},
		{"inventory", strings.Replace(inventory, `"port":22`, `"port":0`, 1)},
		{"inventory", strings.Replace(inventory, `"port":22`, `"port":65536`, 1)},
		{"inventory", strings.Replace(inventory, `"port":22`, `"port":22.0`, 1)},
		{"inventory", strings.Replace(inventory, `"user":"automation"`, `"user":""`, 1)},
		{"inventory", strings.Replace(inventory, `"address":"win.example.test"`, `"address":"${secret}"`, 1)},
		{"inventory", strings.Replace(inventory, `"hosts":["win"]`, `"hosts":null`, 1)},
		{"inventory", strings.Replace(inventory, `"vars":{}`, `"vars":{"ok":false}`, 1)},
		{"inventory", `{"version":1,"hosts":{}}`},
		{"tasks", strings.Replace(tasks, `"module":"command"`, `"module":"unknown"`, 1)},
		{"tasks", strings.Replace(tasks, `"argv":["", "日本語"]`, `"argv":[false]`, 1)},
		{"tasks", strings.Replace(tasks, `"argv":["", "日本語"]`, `"argv":null`, 1)},
		{"tasks", strings.Replace(tasks, `,"argv":["", "日本語"]`, ``, 1)},
		{"tasks", strings.Replace(tasks, `"content":"${message}\n"`, `"content":null`, 1)},
		{"tasks", strings.Replace(tasks, `"args":{"path"`, `"args":{"unexpected":"secret","path"`, 1)},
		{"tasks", `{"version":1,"hosts":"all","tasks":[]}`},
		{"vars-file", `{"ok":null}`}, {"vars-file", `{"ok":1}`}, {"vars-file", `{"1bad":"secret"}`},
	}
	for _, ver := range []string{`"1"`, `1.0`, `1e0`, `2`, `null`, `true`} {
		cases = append(cases, struct{ kind, body string }{"inventory", strings.Replace(inventory, `"version":1`, `"version":`+ver, 1)}, struct{ kind, body string }{"tasks", strings.Replace(tasks, `"version":1`, `"version":`+ver, 1)})
	}
	for i, c := range cases {
		p := write(t, []byte(c.body))
		var e error
		switch c.kind {
		case "inventory":
			_, e = LoadInventory(p)
		case "tasks":
			_, e = LoadTasks(p)
		default:
			_, e = LoadVars(p)
		}
		if e == nil {
			t.Errorf("case %d accepted", i)
		} else if !strings.Contains(e.Error(), c.kind) || !strings.Contains(e.Error(), "$") || strings.Contains(e.Error(), "secret") {
			t.Errorf("unsafe diagnostic: %v", e)
		}
	}
}
func TestWindowsPaths(t *testing.T) {
	for _, s := range []string{"C:\\Windows\\a.exe", "z:\\日本語 folder\\a.txt"} {
		if !WindowsAbsolute(s) {
			t.Error(s)
		}
	}
	for _, s := range []string{"a", "C:a", "\\a", "C:/a", "\\\\server\\share\\a", "\\\\?\\C:\\a", "C:\\a\x00", "C:\\a:b", "C:\\a*"} {
		if WindowsAbsolute(s) {
			t.Error(s)
		}
	}
}
func TestMissingFile(t *testing.T) {
	if _, e := LoadInventory(filepath.Join(t.TempDir(), "SECRET")); e == nil || strings.Contains(e.Error(), "SECRET") {
		t.Fatal(e)
	}
}
func TestSamples(t *testing.T) {
	if _, e := Load("../../examples/inventory.json", "../../examples/tasks.json", "../../examples/vars.json"); e != nil {
		t.Fatal(e)
	}
}
