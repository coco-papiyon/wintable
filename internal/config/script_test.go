package config

import (
	"fmt"
	"testing"
)

func TestScriptSchema(t *testing.T) {
	for _, kind := range []string{"powershell", "shell"} {
		for _, a := range []string{`{}`, `{"script":null}`, `{"script":[]}`, `{"script":""}`, `{"script":" \n"}`, `{"script":"\u0000"}`, `{"script":"exit 0","argv":[]}`, `{"script":"x","script":"y"}`, `{"script":"x"}`} {
			body := fmt.Sprintf(`{"version":1,"hosts":"all","tasks":[{"name":"s","module":%q,"args":%s}]}`, kind, a)
			_, e := LoadTasks(write(t, []byte(body)))
			if (e == nil) != (a == `{"script":"x"}`) {
				t.Fatal(kind, a, e)
			}
		}
	}
}
