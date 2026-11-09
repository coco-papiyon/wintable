package module

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

//go:embed command_linux.py
var linuxCommandSource string

// LinuxCommandWrapper contains only distribution-owned code, never task values.
func LinuxCommandWrapper() string {
	return "/usr/bin/python3 -I -c '" + strings.ReplaceAll(linuxCommandSource, "'", "'\"'\"'") + "'"
}

func DefaultBackends() Backends {
	b := WindowsBackends()
	line := LinuxCommandWrapper()
	b[Key{"linux", "command"}] = Backend{Prepare: PrepareLinuxCommand, Command: line, LaunchLine: line, ValidateLaunch: ValidateLinuxWrapper, Decode: DecodeLinuxCommand}
	line = LinuxFileContentWrapper()
	b[Key{"linux", "file_content"}] = Backend{Prepare: PrepareLinuxFileContent, Command: line, LaunchLine: line, ValidateLaunch: ValidateLinuxWrapper, Decode: DecodeLinuxFileContent}
	return b
}

// DecodeLinuxCommand applies the Linux exit domain before the common strict
// envelope/base64 checks. Windows continues to accept its int32 exit domain.
func DecodeLinuxCommand(r transport.Response, output bool) result.TaskResult {
	var fields map[string]json.RawMessage
	invalid := len(r.Stdout) > transport.WrapperResponseLimit
	if json.Unmarshal(r.Stdout, &fields) == nil {
		if raw, ok := fields["exit_code"]; ok {
			var code int
			invalid = invalid || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &code) != nil || code < -64 || code > 255
		}
	}
	if invalid {
		return result.TaskResult{Status: result.Failed, Error: result.PublicError("task_failed")}
	}
	return DecodeCommand(r, output)
}
