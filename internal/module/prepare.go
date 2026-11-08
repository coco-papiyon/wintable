package module

import (
	"fmt"
	"github.com/coco-papiyon/wintable/internal/config"
	"github.com/coco-papiyon/wintable/internal/transport"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const ContentLimit = 1048576
const JSONLimit = 2097152
const CommandLimit = 32766
const WrapperLimit = 8000

// SizeError intentionally contains no task values or lower-level errors.
type SizeError struct {
	Host                string
	Task, Limit, Actual int
	Internal            bool
}

func (e *SizeError) Error() string {
	return fmt.Sprintf("host=%q task=%d limit=%d actual=%d", e.Host, e.Task, e.Limit, e.Actual)
}
func UTF16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// QuoteCRT implements Windows CRT argument quoting, including trailing backslashes.
func QuoteCRT(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	n := 0
	for _, r := range s {
		if r == '\\' {
			n++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat("\\", 2*n+1))
		} else {
			b.WriteString(strings.Repeat("\\", n))
		}
		n = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat("\\", 2*n))
	b.WriteByte('"')
	return b.String()
}
func CommandLine(exe string, argv []string) string {
	// ProcessStartInfo always quotes FileName, even when it has no spaces.
	a := []string{`"` + exe + `"`}
	for _, s := range argv {
		a = append(a, QuoteCRT(s))
	}
	return strings.Join(a, " ")
}

// ValidateWrapper checks the final distribution launch line INCLUDING the actual
// OpenSSH/cmd prefix and quotes supplied by the wrapper provider. No prefix is guessed.
func ValidateWrapper(finalLine string) error {
	n := UTF16Len(finalLine)
	if n > WrapperLimit {
		return &SizeError{Host: "", Task: 0, Limit: WrapperLimit, Actual: n, Internal: true}
	}
	if finalLine == "" || strings.ContainsRune(finalLine, 0) {
		return fmt.Errorf("internal_error: invalid fixed wrapper")
	}
	return nil
}

// Prepared owns the exact serialized bytes. Request returns a defensive copy;
// transport must send Stdin verbatim and must never marshal arguments again.
type Prepared struct{ data []byte }

func (p Prepared) JSON() []byte { return append([]byte(nil), p.data...) }
func (p Prepared) Request(finalLine string) (transport.Request, error) {
	if e := ValidateWrapper(finalLine); e != nil {
		return transport.Request{}, e
	}
	return transport.Request{Command: finalLine, Stdin: p.JSON(), StdoutLimit: transport.WrapperResponseLimit}, nil
}
func Prepare(host string, task int, kind string, args map[string]any) (Prepared, error) {
	bad := func() (Prepared, error) {
		return Prepared{}, fmt.Errorf("input_error: tasks $: invalid expanded module arguments")
	}
	size := func(n, limit int) error {
		if n > limit {
			return &SizeError{Host: host, Task: task, Limit: limit, Actual: n}
		}
		return nil
	}
	if len(args) != 2 {
		return bad()
	}
	switch kind {
	case "command":
		exe, ok := args["executable"].(string)
		if !ok || !utf8.ValidString(exe) || !config.WindowsAbsolute(exe) {
			return bad()
		}
		base := strings.ToLower(exe[strings.LastIndex(exe, "\\")+1:])
		if !strings.HasSuffix(base, ".exe") || base == "cmd.exe" {
			return bad()
		}
		values, ok := args["argv"].([]any)
		if !ok {
			return bad()
		}
		argv := []string{}
		for _, x := range values {
			s, ok := x.(string)
			if !ok || !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
				return bad()
			}
			argv = append(argv, s)
		}
		if e := size(UTF16Len(CommandLine(exe, argv)), CommandLimit); e != nil {
			return Prepared{}, e
		}
	case "file_content":
		path, ok := args["path"].(string)
		if !ok || !utf8.ValidString(path) || !config.WindowsAbsolute(path) {
			return bad()
		}
		content, ok := args["content"].(string)
		if !ok || !utf8.ValidString(content) {
			return bad()
		}
		if e := size(len(content), ContentLimit); e != nil {
			return Prepared{}, e
		}
	default:
		return bad()
	}
	return Serialize(host, task, kind, args)
}
