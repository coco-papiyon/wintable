//go:build windows

// This rejected prototype is test-only. It documents why the initial local
// commit design cannot yet be used by a product fetch module.
package fetch

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Rejected prototype: directory guards deny deletes, but permit write sharing.
// This is not a safe implementation of the full ancestor/reparse contract.
// The probes isolate the target-file replacement problem only.
type Store struct {
	guards          []windows.Handle
	parent          windows.Handle
	old, temp       *os.File
	name, temporary string
}

func open(p string, access, disposition uint32) (windows.Handle, error) {
	name, e := windows.UTF16PtrFromString(p)
	if e != nil {
		return 0, e
	}
	share := uint32(windows.FILE_SHARE_READ)
	if access == windows.FILE_READ_ATTRIBUTES {
		share |= windows.FILE_SHARE_WRITE
	}
	return windows.CreateFile(name, access, share, nil, disposition, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}
func info(h windows.Handle, dir bool) (windows.ByHandleFileInformation, error) {
	var b windows.ByHandleFileInformation
	e := windows.GetFileInformationByHandle(h, &b)
	if e == nil && (b.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (b.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != dir || (!dir && b.NumberOfLinks != 1)) {
		e = fmt.Errorf("unsafe file")
	}
	return b, e
}

func Begin(ctx context.Context, path string) (*Store, error) {
	s := &Store{name: filepath.Base(path)}
	fail := func() (*Store, error) { s.Close(); return nil, fmt.Errorf("local save unavailable") }
	dir := filepath.Dir(path)
	volume := filepath.VolumeName(dir)
	current := volume + `\`
	parts := []string{}
	if len(dir) > len(current) {
		parts = strings.Split(dir[len(current):], `\`)
	}
	for _, part := range append([]string{""}, parts...) {
		if ctx.Err() != nil {
			return fail()
		}
		if part != "" {
			current = filepath.Join(current, part)
			if e := os.Mkdir(current, 0700); e != nil && !os.IsExist(e) {
				return fail()
			}
		}
		h, e := open(current, windows.FILE_READ_ATTRIBUTES, windows.OPEN_EXISTING)
		if e != nil {
			return fail()
		}
		s.guards = append(s.guards, h)
		if _, e = info(h, true); e != nil {
			return fail()
		}
		s.parent = h
	}
	// Lock the destination by an exclusive create/open without following a link.
	// The lock persists as an empty file; its OS handle, not its existence, is lock ownership.
	lock, e := windows.UTF16PtrFromString(filepath.Join(dir, "."+s.name+".lock"))
	if e != nil {
		return fail()
	}
	lh, e := windows.CreateFile(lock, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return fail()
	}
	s.guards = append(s.guards, lh)
	if _, e = info(lh, false); e != nil {
		return fail()
	}
	h, e := open(path, windows.GENERIC_READ, windows.OPEN_EXISTING)
	if e == nil {
		if _, e = info(h, false); e != nil {
			windows.CloseHandle(h)
			return fail()
		}
		s.old = os.NewFile(uintptr(h), "")
	} else if e != windows.ERROR_FILE_NOT_FOUND {
		return fail()
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return fail()
	}
	s.temporary = filepath.Join(dir, ".wintable-fetch-"+hex.EncodeToString(nonce[:])+".tmp")
	h, e = open(s.temporary, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE, windows.CREATE_NEW)
	if e != nil {
		return fail()
	}
	s.temp = os.NewFile(uintptr(h), "")
	return s, nil
}

func (s *Store) Write(p []byte) (int, error) { return s.temp.Write(p) }

func hash(ctx context.Context, f *os.File) ([32]byte, error) {
	var out [32]byte
	if _, e := f.Seek(0, 0); e != nil {
		return out, e
	}
	h := sha256.New()
	buf := make([]byte, 65536)
	for {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		n, e := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return out, e
		}
	}
	copy(out[:], h.Sum(nil))
	return out, nil
}

// Commit probes FILE_RENAME_INFORMATION_EX with POSIX semantics. It fails for a
// delete-protected old file; allowing delete sharing exposes a replacement race.
func (s *Store) Commit(ctx context.Context, expected [32]byte) (bool, error) {
	got, e := hash(ctx, s.temp)
	if e != nil || got != expected {
		return false, fmt.Errorf("integrity failure")
	}
	if s.old != nil {
		old, e := hash(ctx, s.old)
		if e != nil {
			return false, e
		}
		if old == expected {
			return false, nil
		}
	}
	if e = s.temp.Sync(); e != nil {
		return false, e
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	name, e := windows.UTF16FromString(s.name)
	if e != nil {
		return false, e
	}
	name = name[:len(name)-1]
	// The native structure is pointer-aligned on both Windows architectures.
	type rename struct {
		Flags  uint32
		Root   windows.Handle
		Length uint32
		Name   [1]uint16
	}
	offset := int(unsafe.Offsetof(rename{}.Name))
	b := make([]byte, offset+len(name)*2)
	r := (*rename)(unsafe.Pointer(&b[0]))
	r.Flags = 2 // POSIX_SEMANTICS; no overwrite for a previously absent file.
	if s.old != nil {
		r.Flags |= 1
	}
	r.Root = s.parent
	r.Length = uint32(len(name) * 2)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&b[offset])), len(name)), name)
	var status windows.IO_STATUS_BLOCK
	if e = windows.NtSetInformationFile(windows.Handle(s.temp.Fd()), &status, &b[0], uint32(len(b)), 65); e != nil {
		return false, e
	}
	s.temporary = ""
	return true, nil
}

func (s *Store) Close() error {
	var failure error
	if s.temp != nil {
		if e := s.temp.Close(); e != nil {
			failure = e
		}
		s.temp = nil
	}
	if s.old != nil {
		if e := s.old.Close(); e != nil {
			failure = e
		}
		s.old = nil
	}
	if s.temporary != "" {
		if e := os.Remove(s.temporary); e != nil {
			failure = e
		}
		s.temporary = ""
	}
	for i := len(s.guards) - 1; i >= 0; i-- {
		if e := windows.CloseHandle(s.guards[i]); e != nil {
			failure = e
		}
	}
	s.guards = nil
	if failure != nil {
		return fmt.Errorf("local cleanup failed")
	}
	return nil
}
