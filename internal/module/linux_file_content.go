package module

import (
	_ "embed"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

//go:embed file_content_linux.py
var linuxFileContentSource string

func LinuxFileContentWrapper() string {
	return "/usr/bin/python3 -I -c '" + strings.ReplaceAll(linuxFileContentSource, "'", "'\"'\"'") + "'"
}

func PrepareLinuxFileContent(host string, task int, kind string, args map[string]any) (Prepared, error) {
	bad := func() (Prepared, error) {
		return Prepared{}, fmt.Errorf("input_error: invalid expanded module arguments")
	}
	p, ok := args["path"].(string)
	if !ok || kind != "file_content" || len(args) != 2 || !utf8.ValidString(p) || !strings.HasPrefix(p, "/") || strings.ContainsFunc(p, unicode.IsControl) {
		return bad()
	}
	for _, part := range strings.Split(p[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return bad()
		}
	}
	c, ok := args["content"].(string)
	if !ok || !utf8.ValidString(c) {
		return bad()
	}
	if len(c) > ContentLimit {
		return Prepared{}, &SizeError{Host: host, Task: task, Limit: ContentLimit, Actual: len(c)}
	}
	return Serialize(host, task, kind, args)
}

// DecodeLinuxFileContent uses the file protocol, never the command exit domain.
func DecodeLinuxFileContent(r transport.Response, _ bool) result.TaskResult {
	if len(r.Stdout) > transport.WrapperResponseLimit {
		return result.TaskResult{Status: result.Failed, Error: result.PublicError("task_failed")}
	}
	return DecodeFileContent(r)
}
