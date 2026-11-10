package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"

	"strings"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/transport"
)

func TestPowerShellWindows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	temp := t.TempDir()
	run := func(input []byte) transport.Response {
		c := exec.CommandContext(ctx, "cmd.exe")
		c.SysProcAttr = rawCommand(PowerShellWrapper())
		c.Env = append(os.Environ(), "TEMP="+temp, "TMP="+temp)
		c.Stdin = bytes.NewReader(input)
		var out, diag bytes.Buffer
		c.Stdout = &out
		c.Stderr = &diag
		if e := c.Run(); e != nil {
			t.Fatalf("wrapper: %v %s", e, diag.Bytes())
		}
		zero := 0
		return transport.Response{Stdout: out.Bytes(), Stderr: diag.Bytes(), ExitCode: &zero, Communication: transport.CommunicationComplete}
	}
	for _, tc := range []struct {
		name, script string
		exit         int
		output       string
	}{
		{"unicode-pipe", "[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); '日本語' | ForEach-Object { [Console]::Write($_) }", 0, "日本語"},
		{"exit", "exit 17", 17, ""},
		{"throw", "throw 'private error'", 1, ""},
		{"syntax", "if (", 1, ""},
		{"nonterminating", "Write-Error 'private error'", 0, ""},
		{"native-nonzero", "& $env:ComSpec /c 'exit 7'", 0, ""},
		{"native-explicit", "& $env:ComSpec /c 'exit 7'; exit $LASTEXITCODE", 7, ""},
		{"stdin-EOF", "if ([Console]::In.ReadToEnd() -ne '') {exit 99}", 0, ""},
		{"long", strings.Repeat("# comment\n", 10000) + "exit 0", 0, ""},
		{"capture", "$a='x'*1048577; [Console]::Out.Write($a); [Console]::Error.Write($a)", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, e := PrepareScript("h", 1, "powershell", map[string]any{"script": tc.script})
			if e != nil {
				t.Fatal(e)
			}
			raw := run(p.JSON())
			r := DecodeCommand(raw, true)
			if r.ExitCode == nil || *r.ExitCode != tc.exit {
				t.Fatalf("exit: %+v envelope=%s", r, raw.Stdout)
			}
			if tc.output != "" {
				data, _ := base64.StdEncoding.DecodeString(*r.StdoutBase64)
				if string(data) != tc.output {
					t.Fatalf("output %q", data)
				}
			}
			if tc.name == "capture" && (!r.StdoutTruncated || !r.StderrTruncated) {
				t.Fatal("capture flags")
			}
			hidden := DecodeCommand(raw, false)
			if hidden.StdoutBase64 != nil || hidden.StderrBase64 != nil {
				t.Fatal("output exposed")
			}
			files, e := os.ReadDir(temp)
			if e != nil || len(files) != 0 {
				t.Fatal("temporary scripts leaked", files, e)
			}
		})
	}

	t.Run("receiver-boundaries", func(t *testing.T) {
		for _, n := range []int{ContentLimit - 1, ContentLimit, ContentLimit + 1} {
			data, _ := json.Marshal(map[string]any{"module": "powershell", "args": map[string]any{"script": "#" + strings.Repeat("x", n-1)}})
			r := DecodeCommand(run(data), true)
			if (r.ExitCode != nil) != (n <= ContentLimit) {
				t.Fatal("script boundary", n, r)
			}
		}
		p, _ := PrepareScript("h", 1, "powershell", map[string]any{"script": "exit 0"})
		valid := p.JSON()
		for _, n := range []int{JSONLimit - 1, JSONLimit, JSONLimit + 1} {
			data := append(append([]byte{}, valid...), bytes.Repeat([]byte(" "), n-len(valid))...)
			r := DecodeCommand(run(data), true)
			if (r.ExitCode != nil) != (n <= JSONLimit) {
				t.Fatal("JSON boundary", n, r)
			}
		}
	})
	for _, input := range []string{`{"module":"shell","args":{"script":"exit 0"}}`, `{"module":"powershell","args":{"script":" "}}`, `{"module":"powershell","args":{"script":"exit 0","script":"exit 0"}}`, `{"module":"powershell","args":{"script":"\u0000"}}`} {
		r := DecodeCommand(run([]byte(input)), true)
		if r.Changed == nil || *r.Changed || r.ExitCode != nil {
			t.Fatal("invalid request accepted", r)
		}
	}
	// Child startup failure cleans the script and remains changed=false.
	p, _ := json.Marshal(map[string]any{"module": "powershell", "args": map[string]any{"script": "exit 0"}})
	source := strings.Replace(powershellSource, "powershell.exe", "missing-wintable.exe", 1)
	c := exec.CommandContext(ctx, "cmd.exe")
	c.SysProcAttr = rawCommand(fixedWrapper("using System.Diagnostics;using System.Threading.Tasks;" + receiverSource + source))
	c.Env = append(os.Environ(), "TEMP="+temp, "TMP="+temp)
	c.Stdin = bytes.NewReader(p)
	out, e := c.Output()
	if e != nil {
		t.Fatal(e)
	}
	if string(out) != `{"error":"start_failed"}` {
		t.Fatalf("child start failure %s", out)
	}
	files, e := os.ReadDir(temp)
	if e != nil || len(files) != 0 {
		t.Fatal("startup cleanup", e)
	}
}
