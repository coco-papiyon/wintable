package module

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFileContentWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "日本語 空白 😀.txt")
	run := func(input []byte) transport.Response {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, "cmd.exe")
		c.SysProcAttr = rawCommand(FileContentWrapper())
		c.Stdin = bytes.NewReader(input)
		var o, e bytes.Buffer
		c.Stdout = &o
		c.Stderr = &e
		err := c.Run()
		code := 0
		if err != nil {
			if x, ok := err.(*exec.ExitError); ok {
				code = x.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code, Stdout: o.Bytes(), Stderr: e.Bytes()}
	}
	payload := func(path, content string) []byte {
		b, err := json.Marshal(map[string]any{"module": "file_content", "args": map[string]any{"path": path, "content": content}})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	check := func(path, content string, status result.Status) {
		t.Helper()
		r := run(payload(path, content))
		row := DecodeFileContent(r)
		if row.Status != status {
			t.Fatalf("want %s got %+v outer code=%v stdout=%s stderr=%s", status, row, r.ExitCode, r.Stdout, r.Stderr)
		}
	}
	check(filepath.Join(dir, "empty-new"), "", result.Changed)
	check(filepath.Join(dir, "empty-new"), "", result.OK)
	for _, content := range []string{"日本語\"😀\r\nLF\nno newline", "", "\n", strings.Repeat("a", ContentLimit)} {
		check(path, content, result.Changed)
		before, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		check(path, content, result.OK)
		after, _ := os.Stat(path)
		if !after.ModTime().Equal(before.ModTime()) {
			t.Fatal("equal content modified timestamp")
		}
		b, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(b, []byte(content)) {
			t.Fatal("byte mismatch", err)
		}
	}
	// A BOM is existing data and must be removed to reach the desired bytes.
	if err := os.WriteFile(path, []byte{239, 187, 191, 'x'}, 0600); err != nil {
		t.Fatal(err)
	}
	check(path, "x", result.Changed)
	seed := payload(path, "x")
	for _, n := range []int{JSONLimit - 1, JSONLimit, JSONLimit + 1} {
		r := run(append(append([]byte{}, seed...), bytes.Repeat([]byte(" "), n-len(seed))...))
		row := DecodeFileContent(r)
		if (row.Status == result.OK) != (n <= JSONLimit) {
			t.Fatalf("receiver JSON boundary %d: %+v", n, row)
		}
	}
	check(filepath.Join(dir, "missing", "file"), "x", result.Failed)
	check(dir, "x", result.Failed)
	for _, bad := range []string{filepath.Join(dir, "NUL"), dir + `\..\escape`, dir + `\file.`, dir + `\file `} {
		check(bad, "x", result.Failed)
	}
	before, _ := os.ReadFile(path)
	for _, input := range [][]byte{nil, []byte(`{"module":"file_content","args":{"path":"C:\\a","content":"x","extra":"x"}}`), []byte(`{"module":"file_content","module":"file_content"}`), append(payload(path, "bad"), []byte(` {}`)...), append([]byte{255}, payload(path, "bad")...), payload(path, strings.Repeat("a", ContentLimit+1)), append(payload(path, "bad"), bytes.Repeat([]byte(" "), JSONLimit)...)} {
		r := run(input)
		if row := DecodeFileContent(r); row.Status != result.Failed || row.Changed == nil || *row.Changed {
			t.Fatalf("bad input: %+v %s %s", row, r.Stdout, r.Stderr)
		}
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid input modified target")
	}
	// An open reader denying delete prevents replacement without truncating data.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	check(path, "blocked replacement", result.Failed)
	f.Close()
	after, _ = os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("locked target damaged")
	}
	files, _ := filepath.Glob(filepath.Join(dir, ".wintable-*.tmp"))
	if len(files) != 0 {
		t.Fatalf("temp leak: %v", files)
	}
	// Explicit DACL must survive ReplaceFile, including the same-content path.
	ps := exec.Command("powershell.exe", "-NoProfile", "-Command", `$p=$env:WINTABLE_TEST_PATH;$a=[IO.File]::GetAccessControl($p);$a.SetAccessRuleProtection($true,$true);[IO.File]::SetAccessControl($p,$a);[IO.File]::GetAccessControl($p).Sddl`)
	ps.Env = append(os.Environ(), "WINTABLE_TEST_PATH="+path)
	acl, err := ps.CombinedOutput()
	if err != nil {
		t.Fatalf("ACL setup %v: %s", err, acl)
	}
	check(path, "ACL replacement", result.Changed)
	readACL := exec.Command("powershell.exe", "-NoProfile", "-Command", `[IO.File]::GetAccessControl($env:WINTABLE_TEST_PATH).Sddl`)
	readACL.Env = ps.Env
	got, err := readACL.CombinedOutput()
	if err != nil || !bytes.Equal(acl, got) {
		t.Fatalf("ACL changed: %s / %s: %v", acl, got, err)
	}
	// A junction used as the leaf or any ancestor must not reach its referent.
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "junction")
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, real).CombinedOutput(); err != nil {
		t.Fatalf("junction: %v %s", err, out)
	}
	check(link, "reject leaf", result.Failed)
	check(filepath.Join(link, "child"), "reject ancestor", result.Failed)
	if children, err := os.ReadDir(real); err != nil || len(children) != 0 {
		t.Fatal("junction referent changed", err)
	}
	if link := filepath.Join(dir, "symlink"); os.Symlink(path, link) == nil {
		check(link, "reject symlink", result.Failed)
	} else {
		t.Log("symlink privilege unavailable; junction reparse tests ran")
	}
	// Start four real receivers together. Replacement is last-writer-wins;
	// sharing conflicts may fail, but the published file must be one complete value.
	var wg sync.WaitGroup
	gate := make(chan struct{})
	responses := make(chan transport.Response, 4)
	values := []string{strings.Repeat("a", 100000), strings.Repeat("b", 100000), strings.Repeat("日", 30000), strings.Repeat("\r\n", 40000)}
	for _, v := range values {
		input := payload(path, v)
		wg.Add(1)
		go func() { defer wg.Done(); <-gate; responses <- run(input) }()
	}
	close(gate)
	wg.Wait()
	close(responses)
	success := false
	for r := range responses {
		row := DecodeFileContent(r)
		success = success || row.Status == result.Changed
		if row.Status != result.Changed && row.Status != result.Failed {
			t.Fatal(row)
		}
	}
	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	complete := false
	for _, v := range values {
		complete = complete || string(final) == v
	}
	if !success || !complete {
		t.Fatal("concurrent replacement did not publish a complete value")
	}
	files, _ = filepath.Glob(filepath.Join(dir, ".wintable-*.tmp"))
	if len(files) != 0 {
		t.Fatalf("concurrent temp leak: %v", files)
	}
	t.Logf("real cmd/Windows PowerShell file_content wrapper=%d UTF-16 units; bytes, idempotence, limits, cleanup, locked file, DACL passed", UTF16Len(WrapperLaunchLine(FileContentWrapper())))
}
