package module

import (
	_ "embed"
	"fmt"
	"strings"
	"unicode/utf8"
)

// PrepareScript validates the expanded code before any connection is opened.
func PrepareScript(host string, task int, kind string, args map[string]any) (Prepared, error) {
	s, ok := args["script"].(string)
	if !ok || (kind != "shell" && kind != "powershell") || len(args) != 1 || strings.TrimSpace(s) == "" || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
		return Prepared{}, fmt.Errorf("input_error: invalid expanded script arguments")
	}
	if len(s) > ContentLimit {
		return Prepared{}, &SizeError{Host: host, Task: task, Limit: ContentLimit, Actual: len(s)}
	}
	return Serialize(host, task, kind, args)
}

func LinuxShellWrapper() string {
	return "/usr/bin/python3 -I -c '" + strings.ReplaceAll(linuxShellSource, "'", "'\"'\"'") + "'"
}

func PowerShellWrapper() string {
	return fixedWrapper("using System.Diagnostics;using System.Threading.Tasks;" + receiverSource + powershellSource)
}

//go:embed shell_linux.py
var linuxShellSource string

//go:embed powershell.cs
var powershellSource string
