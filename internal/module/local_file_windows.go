package module

import (
	"context"
	"encoding/base64"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"os"
	"path/filepath"
	"strings"
)

// RunLocalFile uses the same fixed receiver and job lifecycle as remote file.
func RunLocalFile(ctx context.Context, p Prepared) result.TaskResult {
	fields := strings.Fields(FileWrapper("windows"))
	argv := []any{}
	for _, s := range fields[1:] {
		argv = append(argv, s)
	}
	command, e := Prepare("", 0, "command", map[string]any{"executable": filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "argv": argv})
	if e != nil {
		return result.TaskResult{Status: result.Failed, Error: result.PublicError("internal_error")}
	}
	row := runLocalProcess(ctx, command, true, p.JSON())
	if row.ExitCode == nil {
		return row
	}
	var out, errout []byte
	if row.StdoutBase64 != nil {
		out, _ = base64.StdEncoding.DecodeString(*row.StdoutBase64)
	}
	if row.StderrBase64 != nil {
		errout, _ = base64.StdEncoding.DecodeString(*row.StderrBase64)
	}
	return DecodeFileContent(transport.Response{Communication: transport.CommunicationComplete, ExitCode: row.ExitCode, Stdout: out, Stderr: errout, StdoutTruncated: row.StdoutTruncated, StderrTruncated: row.StderrTruncated})
}
