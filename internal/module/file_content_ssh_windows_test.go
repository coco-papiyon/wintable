package module

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileContentSSH(t *testing.T) {
	fixture := os.Getenv("WINTABLE_SSH_FIXTURE")
	if fixture == "" {
		t.Skip("requires disposable Windows SSH fixture")
	}
	raw, err := os.ReadFile(filepath.Join(fixture, "fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Address, User, Container string
		Port                     uint16
	}
	if json.Unmarshal(raw, &cfg) != nil || cfg.Container == "" {
		t.Fatal("invalid fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	session, err := (transport.SSHConnector{}).Connect(ctx, transport.Host{Address: cfg.Address, Port: cfg.Port, User: cfg.User, PrivateKeyFile: filepath.Join(fixture, "client"), KnownHostsFile: filepath.Join(fixture, "known_hosts")})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	path := `C:\fixture\file 日本語.txt`
	payload := func(content string) []byte {
		p, e := Prepare("h", 1, "file_content", map[string]any{"path": path, "content": content})
		if e != nil {
			t.Fatal(e)
		}
		return p.JSON()
	}
	run := func(input []byte) result.TaskResult {
		r, e := session.Run(ctx, transport.Request{Command: FileContentWrapper(), Stdin: input, StdoutLimit: transport.WrapperResponseLimit})
		if e != nil {
			t.Fatal(e)
		}
		row := DecodeFileContent(r)
		if row.Status == result.Failed {
			t.Logf("failed receiver stdout=%s stderr=%s", r.Stdout, r.Stderr)
		}
		return row
	}
	observe := func() struct{ Data, ACL string } {
		out, e := exec.CommandContext(ctx, "docker", "exec", cfg.Container, "powershell.exe", "-NoProfile", "-Command", `$p='C:\fixture\file 日本語.txt';@{Data=[Convert]::ToBase64String([IO.File]::ReadAllBytes($p));ACL=[IO.File]::GetAccessControl($p).Sddl}|ConvertTo-Json -Compress`).CombinedOutput()
		if e != nil {
			t.Fatalf("observe %v: %s", e, out)
		}
		var v struct{ Data, ACL string }
		if json.Unmarshal(bytes.TrimSpace(out), &v) != nil {
			t.Fatalf("observation: %s", out)
		}
		return v
	}
	content := strings.Repeat("x", 10240) + "日本語\"😀\r\nLF\n"
	if row := run(payload(content)); row.Status != result.Changed {
		t.Fatal(row)
	}
	first := observe()
	want := base64.StdEncoding.EncodeToString([]byte(content))
	if first.Data != want {
		t.Fatal("remote bytes mismatch")
	}
	if row := run(payload(content)); row.Status != result.OK {
		t.Fatal(row)
	}
	if row := run(payload("replacement\n")); row.Status != result.Changed {
		t.Fatal(row)
	}
	second := observe()
	// ReplaceFile may set SE_DACL_AUTO_INHERITED without changing ACEs or protection.
	canonicalACL := func(s string) string { return strings.Replace(strings.Replace(s, "D:AI", "D:", 1), "D:PAI", "D:P", 1) }
	if canonicalACL(first.ACL) != canonicalACL(second.ACL) || second.Data != base64.StdEncoding.EncodeToString([]byte("replacement\n")) {
		t.Fatalf("replacement bytes valid=%v; ACL before=%s after=%s", second.Data == base64.StdEncoding.EncodeToString([]byte("replacement\n")), first.ACL, second.ACL)
	}
	if row := run(append(payload("must not write"), []byte(` {}`)...)); row.Status != result.Failed {
		t.Fatal(row)
	}
	if observe() != second {
		t.Fatal("invalid request modified file")
	}
	out, e := exec.CommandContext(ctx, "docker", "exec", cfg.Container, "powershell.exe", "-NoProfile", "-Command", `$PSVersionTable.PSVersion.ToString();cmd /c ver`).CombinedOutput()
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("SSH/cmd file_content: bytes, 10KiB stdin, idempotence, replacement ACL, invalid JSON passed; launch model=%d UTF-16; target=%s", UTF16Len(WrapperLaunchLine(FileContentWrapper())), out)
}
