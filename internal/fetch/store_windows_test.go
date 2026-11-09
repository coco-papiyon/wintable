package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testDir(t *testing.T) string {
	t.Helper()
	base, e := filepath.Abs("../../.cache")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(base, 0700); e != nil {
		t.Fatal(e)
	}
	dir, e := os.MkdirTemp(base, "fetch-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := os.RemoveAll(dir); e != nil {
			t.Error(e)
		}
	})
	return dir
}

func TestStoreCommitFailuresAndRace(t *testing.T) {
	for _, mode := range []string{"flush", "rename", "cancel-before-rename", "hardlink-swap", "symlink-swap"} {
		t.Run(mode, func(t *testing.T) {
			dir := testDir(t)
			path := filepath.Join(dir, "file.bin")
			outside := filepath.Join(dir, "outside.bin")
			os.WriteFile(path, []byte("original"), 0600)
			os.WriteFile(outside, []byte("outside"), 0600)
			s := openStore(t, path)
			s.Write([]byte("downloaded"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "flush":
				s.syncFile = func() error { return io.ErrClosedPipe }
			case "rename":
				p, _ := windows.UTF16PtrFromString(path)
				h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
				if e != nil {
					t.Fatal(e)
				}
				defer windows.CloseHandle(h)
			case "cancel-before-rename":
				s.beforeRename = cancel
			default:
				s.beforeRename = func() {
					if e := os.Remove(path); e != nil {
						t.Fatal(e)
					}
					if mode == "hardlink-swap" {
						if e := os.Link(outside, path); e != nil {
							t.Fatal(e)
						}
					} else {
						if e := os.Symlink(outside, path); e != nil {
							if os.Getenv("WINTABLE_FETCH_REQUIRE_SYMLINK") == "1" {
								t.Fatal("symlink privilege required")
							}
							t.Skip("symlink privilege unavailable; required container probe covers this")
						}
					}
				}
			}
			changed, e := s.Commit(ctx, sha256.Sum256([]byte("downloaded")))
			success := mode == "hardlink-swap" || mode == "symlink-swap"
			if (e == nil) != success || changed != success {
				t.Fatal("unexpected commit", changed, e)
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			want := []byte("original")
			if success {
				want = []byte("downloaded")
			}
			assertContents(t, path, want)
			assertContents(t, outside, []byte("outside"))
			assertClean(t, path)
		})
	}
}
func openStore(t *testing.T, path string) *Store {
	t.Helper()
	s, e := Begin(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	})
	return s
}
func assertContents(t *testing.T, path string, body []byte) {
	t.Helper()
	got, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(got, body) {
		t.Fatal("content mismatch", e)
	}
}
func assertClean(t *testing.T, path string) {
	t.Helper()
	files, e := filepath.Glob(filepath.Join(filepath.Dir(path), ".wintable-fetch-*.tmp"))
	if e != nil || len(files) != 0 {
		t.Fatal("temporary files remain", e)
	}
}

func TestStoreLifecycle(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "nested", "日本語 空白.bin")
	ctx := context.Background()
	for _, body := range [][]byte{{}, []byte("text\r\n\xef\xbb\xbf"), bytes.Repeat([]byte{0, 255, 13, 10}, 400000)} {
		for n := 0; n < 2; n++ {
			s := openStore(t, path)
			if _, e := s.Write(body); e != nil {
				t.Fatal(e)
			}
			changed, e := s.Commit(ctx, sha256.Sum256(body))
			if e != nil || changed != (n == 0) {
				t.Fatalf("changed=%v error=%v", changed, e)
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			assertContents(t, path, body)
			assertClean(t, path)
		}
	}
}
func TestStoreReplacesOversizedExisting(t *testing.T) {
	path := filepath.Join(testDir(t), "file.bin")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	truncateErr := f.Truncate(MaxSize + 1)
	closeErr := f.Close()
	if truncateErr != nil || closeErr != nil {
		t.Fatal(truncateErr, closeErr)
	}
	body := []byte("replacement")
	s := openStore(t, path)
	if _, e := s.Write(body); e != nil {
		t.Fatal(e)
	}
	if changed, e := s.Commit(context.Background(), sha256.Sum256(body)); e != nil || !changed {
		t.Fatalf("changed=%v error=%v", changed, e)
	}
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	assertContents(t, path, body)
	assertClean(t, path)
}

func TestStorePreservesExistingOnFailure(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "file.bin")
	if e := os.WriteFile(path, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"abort", "hash", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			s := openStore(t, path)
			s.Write([]byte("incomplete"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
			}
			if kind != "abort" {
				if changed, e := s.Commit(ctx, sha256.Sum256([]byte("different"))); e == nil || changed {
					t.Fatal("invalid commit accepted")
				}
			}
			if e := s.Close(); e != nil {
				t.Fatal(e)
			}
			assertContents(t, path, []byte("original"))
			assertClean(t, path)
		})
	}
}
func TestStoreLocksAndReparse(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "file.bin")
	s := openStore(t, path)
	if other, e := Begin(context.Background(), path); e == nil {
		other.Close()
		t.Fatal("concurrent destination accepted")
	}
	s.Close()
	outside := filepath.Join(dir, "outside")
	os.Mkdir(outside, 0700)
	junction := filepath.Join(dir, "junction")
	if output, e := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, outside).CombinedOutput(); e != nil {
		t.Fatalf("junction creation failed: %v (%d diagnostic bytes)", e, len(output))
	}
	if e := Preflight(filepath.Join(junction, "file")); e == nil {
		t.Fatal("preflight accepted junction")
	}
	if other, e := Begin(context.Background(), filepath.Join(junction, "file")); e == nil {
		other.Close()
		t.Fatal("junction accepted")
	}
	os.Remove(junction)
	if e := os.WriteFile(path, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Link(path, filepath.Join(dir, "hardlink")); e != nil {
		t.Fatal(e)
	}
	if e := Preflight(path); e == nil {
		t.Fatal("hardlink accepted")
	}
	if other, e := Begin(context.Background(), path); e == nil {
		other.Close()
		t.Fatal("hardlink accepted")
	}
}
func TestStorePermittedReplacement(t *testing.T) {
	dir := testDir(t)
	path := filepath.Join(dir, "file.bin")
	os.WriteFile(path, []byte("original"), 0600)
	s := openStore(t, path)
	s.Write([]byte("downloaded"))
	if e := os.Rename(path, path+".moved"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("competitor"), 0600)
	changed, e := s.Commit(context.Background(), sha256.Sum256([]byte("downloaded")))
	if e != nil || !changed {
		t.Fatal(changed, e)
	}
	s.Close()
	assertContents(t, path, []byte("downloaded"))
	assertContents(t, path+".moved", []byte("original"))
	assertClean(t, path)
}
func TestPathPolicy(t *testing.T) {
	for _, p := range []string{`C:\a`, `D:\日本語 空白.bin`} {
		if !WindowsPath(p) {
			t.Fatal("valid rejected")
		}
	}
	for _, p := range []string{`C:\..\a`, `C:\a.`, `C:\a `, `C:\NUL`, `C:\COM¹`, `C:\file:ads`, `\\host\share`, `C:a`, `C:\a\`, "C:\\x\x00"} {
		if WindowsPath(p) {
			t.Fatalf("invalid accepted %q", p)
		}
	}
	for _, p := range []string{"/", "/x/../y", "/x//y", "/x*", "relative", "/x\n", "/\xff"} {
		if Source("linux", p) {
			t.Fatal("invalid linux accepted")
		}
	}
	root := testDir(t)
	a, e := Destination("host A", "linux", "/same", root)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Destination("host B", "linux", "/same", root)
	if e != nil || strings.EqualFold(a, b) {
		t.Fatal("host collision", e)
	}
	if _, e = os.Stat(filepath.Dir(a)); !os.IsNotExist(e) {
		t.Fatal("preflight created directories")
	}
}
