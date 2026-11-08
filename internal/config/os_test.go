package config

import (
	"strings"
	"testing"
)

func TestHostOS(t *testing.T) {
	for _, v := range []string{`"windows"`, `"linux"`, `""`, `"Windows"`, `"unknown"`, `"${os}"`, `null`, `1`} {
		s := strings.Replace(inventory, `"address":`, `"os":`+v+`,"address":`, 1)
		_, e := LoadInventory(write(t, []byte(s)))
		if (e == nil) != (v == `"windows"` || v == `"linux"`) {
			t.Fatal(v, e)
		}
	}
}
func TestOSSchemaBoundaries(t *testing.T) {
	for _, s := range []string{
		strings.Replace(inventory, `"defaults":{"vars"`, `"defaults":{"os":"linux","vars"`, 1),
		strings.Replace(inventory, `"hosts":["win"]`, `"os":"linux","hosts":["win"]`, 1),
		strings.Replace(inventory, `"address":`, `"os":"windows","os":"linux","address":`, 1),
	} {
		if _, e := LoadInventory(write(t, []byte(s))); e == nil {
			t.Fatal("accepted invalid os")
		}
	}
	m, e := LoadInventory(write(t, []byte(inventory)))
	if e != nil || m["hosts"].(map[string]any)["win"].(map[string]any)["os"] != "windows" {
		t.Fatal(m, e)
	}
	s := strings.Replace(tasks, `C:\\Windows\\System32\\whoami.exe`, `/bin/echo`, 1)
	if _, e := LoadTasks(write(t, []byte(s))); e != nil {
		t.Fatal("target semantics checked in config", e)
	}
}
