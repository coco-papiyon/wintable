package module

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// This test uses only the disposable, authenticated Docker fixture. Holding a
// partial JSON request keeps the real shell alive for independent observation.
func TestCommandSSHLaunchAndDisconnect(t *testing.T)     { testWrapperSSHLaunchAndDisconnect(t, false) }
func TestFileContentSSHLaunchAndDisconnect(t *testing.T) { testWrapperSSHLaunchAndDisconnect(t, true) }
func testWrapperSSHLaunchAndDisconnect(t *testing.T, file bool) {
	command := CommandWrapper()
	suffix := ""
	if file {
		command = FileContentWrapper()
		suffix = "-file"
	}
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
	if err = json.Unmarshal(raw, &cfg); err != nil || cfg.Container == "" {
		t.Fatal("invalid disposable fixture")
	}
	docker := func(script string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, e := exec.CommandContext(ctx, "docker", "exec", cfg.Container, "powershell.exe", "-NoProfile", "-Command", script).CombinedOutput()
		if e != nil {
			t.Fatalf("fixture observation: %v: %s", e, out)
		}
		return out
	}
	// Cancellation does not promise descendant termination. Exclude shells
	// already present before this test, rather than attributing them to its exec.
	var previous []uint32
	if err = json.Unmarshal(bytes.TrimSpace(docker(`$ids=@(Get-CimInstance Win32_Process -Filter "Name='cmd.exe'" | Select-Object -ExpandProperty ProcessId);ConvertTo-Json -InputObject $ids -Compress`)), &previous); err != nil {
		t.Fatal(err)
	}
	previousIDs := make([]string, len(previous))
	for i, pid := range previous {
		previousIDs[i] = strconv.FormatUint(uint64(pid), 10)
	}
	key, err := os.ReadFile(filepath.Join(fixture, "client"))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	verify, err := knownhosts.New(filepath.Join(fixture, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(cfg.Address, strconv.Itoa(int(cfg.Port))), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	c, ch, req, err := ssh.NewClientConn(conn, net.JoinHostPort(cfg.Address, strconv.Itoa(int(cfg.Port))), &ssh.ClientConfig{User: cfg.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: verify})
	if err != nil {
		t.Fatal(err)
	}
	client := ssh.NewClient(c, ch, req)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err = session.Start(command); err != nil {
		t.Fatal(err)
	}
	marker := `C:\fixture\partial-request-marker` + suffix
	input, _ := json.Marshal(map[string]any{"module": "command", "args": map[string]any{"executable": `C:\fixture\日本語 空白.exe`, "argv": []string{"--marker", marker}}})
	if file {
		input, _ = json.Marshal(map[string]any{"module": "file_content", "args": map[string]any{"path": marker, "content": "日本語"}})
	}
	if _, err = stdin.Write(input[:len(input)-1]); err != nil {
		t.Fatal(err)
	}
	// CIM observes the command line supplied to the running process, not sshd's
	// truncated debug log or a reconstruction from the exec request.
	out := docker(`$ErrorActionPreference='Stop';$before=@(` + strings.Join(previousIDs, ",") + `);$p=$null;for($i=0;$i -lt 40;$i++){ $p=@(Get-CimInstance Win32_Process -Filter "Name='cmd.exe'" | Where-Object {$before -notcontains $_.ProcessId -and $_.CommandLine.Contains('` + command[len(command)/2:len(command)/2+80] + `')});if($p.Count){break};Start-Sleep -Milliseconds 250 };if($p.Count -ne 1){throw 'expected one wrapper shell'};if(Test-Path '` + marker + `'){throw 'executed before EOF'};$p[0].CommandLine | ConvertTo-Json -Compress`)
	var line string
	if err = json.Unmarshal(bytes.TrimSpace(out), &line); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fixture, "wrapper-launch"+suffix+".json"), out, 0600); err != nil {
		t.Fatal(err)
	}
	want := WrapperLaunchLine(command)
	t.Logf("observed shell UTF-16 units=%d, model=%d", UTF16Len(line), UTF16Len(want))
	const shell = `"C:\Windows\System32\cmd.exe"`
	if len(line) < len(shell) || !strings.EqualFold(line[:len(shell)], shell) || line[len(shell):] != want[len(shell):] {
		t.Errorf("actual shell differs: prefix=%q suffix=%q", line[:min(40, len(line))], line[max(0, len(line)-12):])
	}
	// A second exec supplies valid UTF-8 JSON in seven-byte SSH writes. The
	// Japanese path spans writes; only an explicit EOF permits execution.
	fragmented, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer fragmented.Close()
	pipe, err := fragmented.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var body, diag bytes.Buffer
	fragmented.Stdout = &body
	fragmented.Stderr = &diag
	if err = fragmented.Start(command); err != nil {
		t.Fatal(err)
	}
	valid, _ := json.Marshal(map[string]any{"module": "command", "args": map[string]any{"executable": `C:\fixture\日本語 空白.exe`, "argv": []string{"--assert-absent", marker}}})
	if file {
		valid, _ = json.Marshal(map[string]any{"module": "file_content", "args": map[string]any{"path": marker + "-valid", "content": "日本語😀\r\n"}})
	}
	for n := 0; n < len(valid); n += 7 {
		if _, err = pipe.Write(valid[n:min(n+7, len(valid))]); err != nil {
			t.Fatal(err)
		}
	}
	if err = pipe.Close(); err != nil {
		t.Fatal(err)
	}
	if err = fragmented.Wait(); err != nil {
		t.Fatal(err)
	}
	code := 0
	row := DecodeCommand(transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code, Stdout: body.Bytes(), Stderr: diag.Bytes()}, false)
	if file {
		row = DecodeFileContent(transport.Response{Communication: transport.CommunicationComplete, ExitCode: &code, Stdout: body.Bytes(), Stderr: diag.Bytes()})
	}
	if row.Status != result.Changed {
		t.Fatalf("fragmented UTF-8 request: %+v", row)
	}
	t.Log("seven-byte SSH writes and explicit EOF: valid request executed; pending request still has no side effect")
	// Cut the TCP connection while JSON is incomplete and EOF was not sent.
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case err = <-done:
		if err == nil {
			t.Error("disconnect unexpectedly succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("disconnect hung")
	}
	docker(`Start-Sleep -Seconds 2;if(Test-Path '` + marker + `'){throw 'partial request executed after disconnect'}`)
	t.Log("partial JSON disconnect: returned within 10 seconds; no child side effect before/after disconnect")
}
