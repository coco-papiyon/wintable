package module

import (
	"encoding/json"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Key struct{ OS, Module string }
type Backend struct {
	Target              Key
	Prepare             func(string, int, string, map[string]any) (Prepared, error)
	Command, LaunchLine string
	ValidateLaunch      func(string) error
	Decode              func(transport.Response, bool) result.TaskResult
}
type Backends map[Key]Backend

func (b Backend) Check() error {
	if b.Prepare == nil || b.Decode == nil || b.ValidateLaunch == nil || b.Command == "" {
		return fmt.Errorf("internal_error: incomplete backend")
	}
	if e := b.ValidateLaunch(b.Command); e != nil {
		return e
	}
	return b.ValidateLaunch(b.LaunchLine)
}
func (b Backend) Request(p Prepared) transport.Request {
	return transport.Request{Command: b.Command, Stdin: p.JSON(), StdoutLimit: transport.WrapperResponseLimit}
}
func WindowsBackends() Backends {
	r := Backends{}
	for _, k := range []string{"command", "file_content"} {
		c := CommandWrapper()
		d := DecodeCommand
		if k == "file_content" {
			c = FileContentWrapper()
			d = func(r transport.Response, _ bool) result.TaskResult { return DecodeFileContent(r) }
		}
		r[Key{"windows", k}] = Backend{Prepare: Prepare, Command: c, LaunchLine: WrapperLaunchLine(c), ValidateLaunch: ValidateWrapper, Decode: d}
	}
	return r
}

const LinuxArgumentLimit = 32 * 1024
const LinuxArgumentsLimit = 64 * 1024
const LinuxArgvLimit = 4096
const LinuxWrapperLimit = 8 * 1024

func ValidateLinuxWrapper(s string) error {
	if len(s) > LinuxWrapperLimit {
		return &SizeError{Limit: LinuxWrapperLimit, Actual: len(s), Internal: true}
	}
	if s == "" || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
		return fmt.Errorf("internal_error: invalid fixed wrapper")
	}
	return nil
}

// PrepareLinuxCommand validates and serializes the Linux command request.
func PrepareLinuxCommand(host string, task int, kind string, args map[string]any) (Prepared, error) {
	bad := func() (Prepared, error) {
		return Prepared{}, fmt.Errorf("input_error: invalid expanded module arguments")
	}
	exe, ok := args["executable"].(string)
	if !ok || kind != "command" || len(args) != 2 || !strings.HasPrefix(exe, "/") || strings.HasSuffix(exe, "/") || strings.ContainsFunc(exe, unicode.IsControl) {
		return bad()
	}
	for _, p := range strings.Split(exe, "/") {
		if p == "." || p == ".." {
			return bad()
		}
	}
	argv, ok := args["argv"].([]any)
	if !ok {
		return bad()
	}
	size := func(n, l int) error {
		if n > l {
			return &SizeError{Host: host, Task: task, Limit: l, Actual: n}
		}
		return nil
	}
	if e := size(len(argv), LinuxArgvLimit); e != nil {
		return Prepared{}, e
	}
	total := 0
	for _, v := range append([]any{exe}, argv...) {
		s, ok := v.(string)
		if !ok || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return bad()
		}
		if e := size(len(s), LinuxArgumentLimit); e != nil {
			return Prepared{}, e
		}
		total += len(s) + 1
	}
	if e := size(total, LinuxArgumentsLimit); e != nil {
		return Prepared{}, e
	}
	return Serialize(host, task, kind, args)
}
func Serialize(host string, task int, kind string, args map[string]any) (Prepared, error) {
	data, e := json.Marshal(struct {
		Module string         `json:"module"`
		Args   map[string]any `json:"args"`
	}{kind, args})
	if e != nil {
		return Prepared{}, fmt.Errorf("input_error: invalid module arguments")
	}
	if len(data) > JSONLimit {
		return Prepared{}, &SizeError{Host: host, Task: task, Limit: JSONLimit, Actual: len(data)}
	}
	return Prepared{data: data}, nil
}
