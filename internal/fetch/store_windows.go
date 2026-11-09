package fetch

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

type Store struct {
	dirs         []windows.Handle
	parent, lock windows.Handle
	temp         *os.File
	name         string
	committed    bool
	beforeRename func() // deterministic interleaving in package tests; nil in product.
	syncFile     func() error
}

func safeInfo(h windows.Handle, dir bool) error {
	var b windows.ByHandleFileInformation
	if e := windows.GetFileInformationByHandle(h, &b); e != nil {
		return e
	}
	if b.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (b.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != dir || !dir && b.NumberOfLinks != 1 {
		return fmt.Errorf("unsafe file")
	}
	return nil
}

// Each child is opened relative to an already checked directory handle. No
// intermediate path is traversed by a string-based filesystem operation.
func relative(parent windows.Handle, name string, dir, create bool, access, share uint32, sd *windows.SECURITY_DESCRIPTOR) (windows.Handle, error) {
	n, e := windows.NewNTUnicodeString(name)
	if e != nil {
		return 0, e
	}
	oa := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: parent, ObjectName: n, Attributes: windows.OBJ_CASE_INSENSITIVE, SecurityDescriptor: sd}
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	if dir {
		options |= windows.FILE_DIRECTORY_FILE
	} else {
		options |= windows.FILE_NON_DIRECTORY_FILE
	}
	disposition := uint32(windows.FILE_OPEN)
	if create {
		disposition = windows.FILE_OPEN_IF
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	e = windows.NtCreateFile(&h, access|windows.SYNCHRONIZE, &oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, share, disposition, options, 0, 0)
	runtime.KeepAlive(n)
	runtime.KeepAlive(sd)
	if e != nil {
		return 0, e
	}
	if e = safeInfo(h, dir); e != nil {
		windows.CloseHandle(h)
		return 0, e
	}
	return h, nil
}
func root(path string) (windows.Handle, error) {
	p, e := windows.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	if e != nil {
		return 0, e
	}
	h, e := windows.CreateFile(p, windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return 0, e
	}
	if e = safeInfo(h, true); e != nil {
		windows.CloseHandle(h)
		return 0, e
	}
	var filesystem [32]uint16
	if e = windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, nil, &filesystem[0], uint32(len(filesystem))); e != nil || windows.UTF16ToString(filesystem[:]) != "NTFS" {
		windows.CloseHandle(h)
		return 0, fmt.Errorf("NTFS required")
	}
	return h, nil
}
func missing(e error) bool {
	return e == windows.STATUS_OBJECT_NAME_NOT_FOUND || e == windows.STATUS_OBJECT_PATH_NOT_FOUND
}

// Preflight inspects existing ancestors and final files without creating anything.
func Preflight(path string) error {
	if !WindowsPath(path) {
		return fmt.Errorf("invalid destination")
	}
	h, e := root(path)
	if e != nil {
		return e
	}
	held := []windows.Handle{h}
	defer func() {
		for i := len(held) - 1; i >= 0; i-- {
			windows.CloseHandle(held[i])
		}
	}()
	parts := strings.Split(path[3:], `\`)
	for i, part := range parts {
		dir := i < len(parts)-1
		access := uint32(windows.FILE_READ_ATTRIBUTES)
		share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
		if !dir {
			share |= windows.FILE_SHARE_DELETE
		}
		child, e := relative(h, part, dir, false, access, share, nil)
		if missing(e) {
			return nil
		}
		if e != nil {
			return e
		}
		held = append(held, child)
		h = child
	}
	return nil
}

func Begin(ctx context.Context, path string) (*Store, error) {
	s := &Store{name: filepath.Base(path)}
	fail := func() (*Store, error) { _ = s.Close(); return nil, fmt.Errorf("local save failed") }
	if !WindowsPath(path) {
		return fail()
	}
	h, e := root(path)
	if e != nil {
		return fail()
	}
	s.dirs = append(s.dirs, h)
	for _, part := range strings.Split(filepath.Dir(path)[3:], `\`) {
		if ctx.Err() != nil {
			return fail()
		}
		h, e = relative(h, part, true, true, windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil)
		if e != nil {
			return fail()
		}
		s.dirs = append(s.dirs, h)
	}
	s.parent = h
	token := windows.GetCurrentProcessToken()
	user, e := token.GetTokenUser()
	if e != nil {
		return fail()
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;" + user.User.Sid.String() + ")")
	if e != nil {
		return fail()
	}
	s.lock, e = relative(h, "."+s.name+".lock", false, true, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, sd)
	if e != nil {
		return fail()
	}
	// Check the current target without following links. Third-party replacement
	// after this check is allowed; rename replaces the entry, never its link target.
	old, e := relative(h, s.name, false, false, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil)
	if e == nil {
		windows.CloseHandle(old)
	} else if !missing(e) {
		return fail()
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return fail()
	}
	// FILE_CREATE, not OPEN_IF: an existing entry is never reused as the temp.
	n, e := windows.NewNTUnicodeString(".wintable-fetch-" + hex.EncodeToString(nonce[:]) + ".tmp")
	if e != nil {
		return fail()
	}
	oa := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: h, ObjectName: n, Attributes: windows.OBJ_CASE_INSENSITIVE, SecurityDescriptor: sd}
	var temp windows.Handle
	var status windows.IO_STATUS_BLOCK
	e = windows.NtCreateFile(&temp, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE|windows.SYNCHRONIZE, &oa, &status, nil, windows.FILE_ATTRIBUTE_TEMPORARY, 0, windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(n)
	runtime.KeepAlive(sd)
	if e != nil {
		return fail()
	}
	s.temp = os.NewFile(uintptr(temp), "")
	return s, nil
}
func (s *Store) Write(p []byte) (int, error) { return s.temp.Write(p) }
func hashFile(ctx context.Context, f *os.File) ([32]byte, int64, error) {
	var digest [32]byte
	if _, e := f.Seek(0, 0); e != nil {
		return digest, 0, e
	}
	h := sha256.New()
	buf := make([]byte, 65536)
	var total int64
	for {
		if ctx.Err() != nil {
			return digest, total, ctx.Err()
		}
		n, e := f.Read(buf)
		total += int64(n)
		if total > MaxSize {
			return digest, total, fmt.Errorf("size limit")
		}
		if n > 0 {
			h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return digest, total, e
		}
	}
	copy(digest[:], h.Sum(nil))
	return digest, total, nil
}
func (s *Store) Commit(ctx context.Context, expected [32]byte) (bool, error) {
	got, size, e := hashFile(ctx, s.temp)
	if e != nil || got != expected {
		return false, fmt.Errorf("integrity failure")
	}
	old, e := relative(s.parent, s.name, false, false, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil)
	if e == nil {
		f := os.NewFile(uintptr(old), "")
		digest, n, readErr := hashFile(ctx, f)
		closeErr := f.Close()
		if readErr != nil || closeErr != nil {
			return false, fmt.Errorf("comparison failed")
		}
		if n == size && digest == expected {
			return false, nil
		}
	} else if !missing(e) {
		return false, fmt.Errorf("target unavailable")
	}
	sync := s.temp.Sync
	if s.syncFile != nil {
		sync = s.syncFile
	}
	if e = sync(); e != nil {
		return false, e
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if s.beforeRename != nil {
		s.beforeRename()
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	var status windows.IO_STATUS_BLOCK
	name, _ := windows.UTF16FromString(s.name)
	name = name[:len(name)-1]
	type rename struct {
		Flags  uint32
		Root   windows.Handle
		Length uint32
		Name   [1]uint16
	}
	offset := int(unsafe.Offsetof(rename{}.Name))
	b := make([]byte, offset+len(name)*2)
	r := (*rename)(unsafe.Pointer(&b[0]))
	r.Flags = 3
	r.Root = s.parent
	r.Length = uint32(len(name) * 2)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&b[offset])), len(name)), name)
	e = windows.NtSetInformationFile(windows.Handle(s.temp.Fd()), &status, &b[0], uint32(len(b)), 65)
	if e != nil {
		return false, e
	}
	s.committed = true
	return true, nil
}
func (s *Store) Close() error {
	var failure error
	if s.temp != nil {
		if !s.committed {
			flag := byte(1)
			var status windows.IO_STATUS_BLOCK
			failure = windows.NtSetInformationFile(windows.Handle(s.temp.Fd()), &status, &flag, 1, 13)
		}
		if e := s.temp.Close(); e != nil {
			failure = e
		}
		s.temp = nil
	}
	if s.lock != 0 {
		if e := windows.CloseHandle(s.lock); e != nil {
			failure = e
		}
		s.lock = 0
	}
	for i := len(s.dirs) - 1; i >= 0; i-- {
		if e := windows.CloseHandle(s.dirs[i]); e != nil {
			failure = e
		}
	}
	s.dirs = nil
	if failure != nil {
		return fmt.Errorf("local cleanup failed")
	}
	return nil
}
