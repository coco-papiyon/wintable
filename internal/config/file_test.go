package config

import (
	"strings"
	"testing"
)

func TestFileSchema(t *testing.T) {
	base := `{"version":1,"hosts":"all","tasks":[{"name":"directory","module":"file","delegate_to":"localhost","args":{"path":"C:\\Temp\\data","state":"directory"}}]}`
	if _, e := LoadTasks(write(t, []byte(base))); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{strings.Replace(base, `"state":"directory"`, `"state":"absent"`, 1), strings.Replace(base, `"state":"directory"`, `"state":null`, 1), strings.Replace(base, `"state":"directory"`, `"state":"directory","extra":"x"`, 1), strings.Replace(base, `"state":"directory"`, `"state":"directory","state":"directory"`, 1), strings.Replace(base, `"path":"C:\\Temp\\data"`, `"path":1`, 1)} {
		if _, e := LoadTasks(write(t, []byte(s))); e == nil {
			t.Fatal("accepted invalid schema")
		}
	}
}
