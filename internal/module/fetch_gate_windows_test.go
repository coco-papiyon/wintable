package module

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFetchWindowsFeasibility(t *testing.T) {
	if os.Getenv("WINTABLE_FETCH_FEASIBILITY") != "1" {
		t.Skip("opt-in rejected-design probe; see tests/fetch-feasibility/README.md")
	}
	source, err := os.ReadFile("../../tests/fetch-feasibility/sender.cs")
	if err != nil {
		t.Fatal(err)
	}
	command := fixedWrapper(receiverSource + string(source))
	t.Logf("fixed launch line: %d UTF-16 units (limit %d)", UTF16Len(WrapperLaunchLine(command)), WrapperLimit)
	if err := ValidateWrapper(WrapperLaunchLine(command)); err != nil {
		t.Fatal(err)
	}
	base, _ := filepath.Abs("../../.cache")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "fetch-gate-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "日本語 空白.bin")
	data := bytes.Repeat([]byte{0, 255, 13, 10, 239, 187, 191}, 200000)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"module": "fetch", "args": map[string]any{"src": path}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "cmd.exe")
	c.SysProcAttr = rawCommand(command)
	c.Stdin = bytes.NewReader(input)
	var out, stderr bytes.Buffer
	c.Stdout = &out
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		t.Fatalf("sender exit: %v, diagnostic bytes: %d", err, stderr.Len())
	}
	frame := out.Bytes()
	hash := sha256.Sum256(data)
	if len(frame) != 12+len(data)+32 || string(frame[:4]) != "WTF1" || binary.LittleEndian.Uint64(frame[4:12]) != uint64(len(data)) || !bytes.Equal(frame[12:12+len(data)], data) || !bytes.Equal(frame[12+len(data):], hash[:]) {
		t.Fatalf("invalid transfer: bytes=%d expected=%d diagnostic bytes=%d", len(frame), 12+len(data)+32, stderr.Len())
	}
}
