package module

import (
	_ "embed"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/config"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"strings"
	"unicode"
	"unicode/utf8"
)

//go:embed file_linux.py
var fileLinuxSource string

//go:embed file.cs
var fileSource string

func FileWrapper(target string) string {
	if target == "linux" {
		return "/usr/bin/python3 -I -c '" + strings.ReplaceAll(fileLinuxSource, "'", "'\"'\"'") + "'"
	}
	return fixedWrapper(receiverSource + fileSource)
}

func validDirectoryPath(target, p string) bool {
	if !utf8.ValidString(p) || strings.ContainsFunc(p, unicode.IsControl) {
		return false
	}
	sep := "/"
	if target == "windows" {
		if !config.WindowsAbsolute(p) {
			return false
		}
		p = p[3:]
		sep = "\\"
	} else {
		if !strings.HasPrefix(p, "/") {
			return false
		}
		p = p[1:]
	}
	for _, c := range strings.Split(p, sep) {
		if c == "" || c == "." || c == ".." {
			return false
		}
		if target == "windows" {
			if strings.HasSuffix(c, ".") || strings.HasSuffix(c, " ") {
				return false
			}
			n := strings.ToUpper(strings.Split(c, ".")[0])
			if n == "CON" || n == "PRN" || n == "AUX" || n == "NUL" || (len(n) == 4 && (strings.HasPrefix(n, "COM") || strings.HasPrefix(n, "LPT")) && strings.ContainsRune("123456789¹²³", rune(n[3]))) || n == "COM¹" || n == "COM²" || n == "COM³" || n == "LPT¹" || n == "LPT²" || n == "LPT³" {
				return false
			}
		}
	}
	return true
}

func FileBackend(target string) Backend {
	line := FileWrapper(target)
	launch := line
	validate := ValidateLinuxWrapper
	if target == "windows" {
		launch = WrapperLaunchLine(line)
		validate = ValidateWrapper
	}
	return Backend{Command: line, LaunchLine: launch, ValidateLaunch: validate,
		Prepare: func(host string, task int, kind string, args map[string]any) (Prepared, error) {
			p, ok := args["path"].(string)
			if kind != "file" || len(args) != 2 || !ok || args["state"] != "directory" || !validDirectoryPath(target, p) {
				return Prepared{}, fmt.Errorf("input_error: invalid file arguments")
			}
			return Serialize(host, task, kind, args)
		}, Decode: func(r transport.Response, _ bool) result.TaskResult { return DecodeFileContent(r) }}
}
