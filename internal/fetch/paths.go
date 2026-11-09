// Package fetch owns the control-side receive transaction and path policy.
package fetch

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxSize int64 = 1 << 30

func WindowsPath(p string) bool {
	if len(p) < 4 || p[1] != ':' || p[2] != '\\' || !((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) || strings.ContainsAny(p[3:], `<>:"|?*/`) || strings.ContainsFunc(p, unicode.IsControl) || !utf8.ValidString(p) {
		return false
	}
	for _, c := range strings.Split(p[3:], `\`) {
		if c == "" || c == "." || c == ".." || strings.HasSuffix(c, ".") || strings.HasSuffix(c, " ") {
			return false
		}
		n := strings.ToUpper(strings.Split(c, ".")[0])
		runes := []rune(n)
		if n == "CON" || n == "PRN" || n == "AUX" || n == "NUL" || len(runes) == 4 && (strings.HasPrefix(n, "COM") || strings.HasPrefix(n, "LPT")) && strings.ContainsRune("123456789¹²³", runes[3]) {
			return false
		}
	}
	return len(p) <= 240 // leaves room for Win32 lock/temp paths; no long-path opt-in.
}
func Source(target, p string) bool {
	if target == "windows" {
		return WindowsPath(p)
	}
	if target != "linux" || !strings.HasPrefix(p, "/") || len(p) > 4096 || !utf8.ValidString(p) || strings.ContainsFunc(p, unicode.IsControl) || strings.ContainsAny(p, "*?[") {
		return false
	}
	for _, c := range strings.Split(p[1:], "/") {
		if c == "" || c == "." || c == ".." {
			return false
		}
	}
	return true
}
func Destination(host, target, src, dest string) (string, error) {
	bad := fmt.Errorf("input_error: invalid fetch paths")
	if runtime.GOOS != "windows" || !Source(target, src) || dest == "" || strings.ContainsAny(dest, "/") || strings.ContainsFunc(dest, unicode.IsControl) {
		return "", bad
	}
	// Reject dot segments before cleaning or resolving the relative root.
	for _, c := range strings.Split(dest, `\`) {
		if c == "." || c == ".." {
			return "", bad
		}
	}
	if !filepath.IsAbs(dest) && (strings.Contains(dest, ":") || strings.HasPrefix(dest, `\`)) {
		return "", bad
	}
	root, e := filepath.Abs(dest)
	if e != nil || (!WindowsPath(root) && root != filepath.VolumeName(root)+`\`) {
		return "", bad
	}
	normalized := src
	if target == "windows" {
		normalized = strings.ToLower(src)
	}
	h := sha256.Sum256([]byte(host))
	s := sha256.Sum256([]byte(target + "\x00" + normalized))
	path := filepath.Join(root, fmt.Sprintf("h-%x", h), fmt.Sprintf("s-%x.bin", s))
	if !WindowsPath(path) {
		return "", bad
	}
	if e = Preflight(path); e != nil {
		return "", bad
	}
	return path, nil
}
