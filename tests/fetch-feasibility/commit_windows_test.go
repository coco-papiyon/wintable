package fetch

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func destination(t *testing.T) string {
	t.Helper()
	if os.Getenv("WINTABLE_FETCH_FEASIBILITY") != "1" {
		t.Skip("opt-in rejected-design probe; see README.md")
	}
	base, e := filepath.Abs("../../.cache")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(base, 0700); e != nil {
		t.Fatal(e)
	}
	dir, e := os.MkdirTemp(base, "fetch-store-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := os.RemoveAll(dir); e != nil {
			t.Error(e)
		}
	})
	return filepath.Join(dir, "nested", "file.bin")
}

func stage(t *testing.T, path, body string) *Store {
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
	if _, e = s.Write([]byte(body)); e != nil {
		t.Fatal(e)
	}
	return s
}

func TestProtectedReplacementIsRejected(t *testing.T) {
	path := destination(t)
	ctx := context.Background()
	for i, body := range []string{"original", "original", "updated"} {
		s := stage(t, path, body)
		changed, e := s.Commit(ctx, sha256.Sum256([]byte(body)))
		if i == 2 {
			if !errors.Is(e, windows.STATUS_SHARING_VIOLATION) || changed {
				t.Fatalf("expected sharing violation, changed=%v err=%v", changed, e)
			}
			t.Log("DELETE sharing denied: new and equal succeed; update rejected with STATUS_SHARING_VIOLATION")
		} else if e != nil || changed != (i == 0) {
			t.Fatalf("step %d changed=%v err=%v", i, changed, e)
		}
		if e := s.Close(); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(path)
		if e != nil || string(b) != "original" {
			t.Fatal("original not preserved", e)
		}
	}
}

// A deterministic interleaving, not a probabilistic stress test: the competing
// rename occurs after the verified old handle is opened, before the final rename.
func TestDeleteSharedReplacementLosesIdentityProtection(t *testing.T) {
	path := destination(t)
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	s := stage(t, path, "downloaded")
	if e := s.old.Close(); e != nil {
		t.Fatal(e)
	}
	s.old = nil
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		t.Fatal(e)
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		t.Fatal(e)
	}
	s.old = os.NewFile(uintptr(h), "")
	if e = os.Rename(path, path+".moved"); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte("competitor"), 0600); e != nil {
		t.Fatal(e)
	}
	changed, e := s.Commit(context.Background(), sha256.Sum256([]byte("downloaded")))
	if e != nil || !changed {
		t.Fatalf("expected prototype to expose replacement race, changed=%v err=%v", changed, e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(path)
	if e != nil || string(got) != "downloaded" {
		t.Fatal("unexpected final content", e)
	}
	t.Log("DELETE sharing allowed: original handle remains valid, but a competitor's replacement is overwritten")
}
