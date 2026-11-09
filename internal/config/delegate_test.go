package config

import (
	"strings"
	"testing"
)

func TestDelegationSchema(t *testing.T) {
	base := `{"version":1,"hosts":"all","tasks":[{"name":"local","module":"command","args":{"executable":"C:\\Windows\\System32\\hostname.exe","argv":[]}ATTR}]}`
	for _, tc := range []struct {
		attr string
		ok   bool
	}{
		{``, true}, {`,"delegate_to":"localhost"`, true}, {`,"become":false`, true},
		{`,"delegate_to":"localhost","become":false`, true}, {`,"delegate_to":"remote"`, false},
		{`,"delegate_to":false`, false}, {`,"delegate_to":null`, false}, {`,"become":true`, false},
		{`,"become":"false"`, false}, {`,"become":null`, false},
	} {
		_, err := LoadTasks(write(t, []byte(strings.ReplaceAll(base, "ATTR", tc.attr))))
		if (err == nil) != tc.ok {
			t.Fatalf("%s: %v", tc.attr, err)
		}
	}
	for _, mod := range []string{"file_content", "fetch"} {
		s := strings.ReplaceAll(strings.ReplaceAll(base, "ATTR", `,"delegate_to":"localhost"`), `"module":"command"`, `"module":"`+mod+`"`)
		if _, err := LoadTasks(write(t, []byte(s))); err == nil {
			t.Fatal(mod)
		}
	}
	local := write(t, []byte(strings.ReplaceAll(base, "ATTR", `,"delegate_to":"localhost"`)))
	inv := write(t, []byte(`{"version":1,"hosts":{"original":{"os":"linux"}}}`))
	if _, err := Load(inv, local, ""); err != nil {
		t.Fatal(err)
	}
	remote := write(t, []byte(strings.ReplaceAll(base, "ATTR", ``)))
	if _, err := Load(inv, remote, ""); err == nil {
		t.Fatal("remote task must require connection fields")
	}
}
