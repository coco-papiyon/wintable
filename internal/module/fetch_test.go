package module

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"github.com/coco-papiyon/wintable/internal/fetch"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
	"hash"
	"io"
	"testing"
)

func frame(body []byte) []byte {
	var b bytes.Buffer
	b.WriteString("WTF1")
	binary.Write(&b, binary.LittleEndian, uint64(len(body)))
	b.Write(body)
	h := sha256.Sum256(body)
	b.Write(h[:])
	return b.Bytes()
}

// Generate the size boundary without allocating a file-sized byte slice.
type generatedFrame struct {
	left    int64
	h       hash.Hash
	trailer []byte
}

func (g *generatedFrame) Read(p []byte) (int, error) {
	if g.left > 0 {
		n := int64(len(p))
		if n > g.left {
			n = g.left
		}
		clear(p[:n])
		g.h.Write(p[:n])
		g.left -= n
		return int(n), nil
	}
	if g.trailer == nil {
		g.trailer = g.h.Sum(nil)
	}
	if len(g.trailer) == 0 {
		return 0, io.EOF
	}
	n := copy(p, g.trailer)
	g.trailer = g.trailer[n:]
	return n, nil
}
func TestFetchSizeBoundary(t *testing.T) {
	for _, size := range []int64{fetch.MaxSize - 1, fetch.MaxSize, fetch.MaxSize + 1} {
		var header bytes.Buffer
		header.WriteString("WTF1")
		binary.Write(&header, binary.LittleEndian, uint64(size))
		generated := &generatedFrame{left: size, h: sha256.New()}
		_, e := Receive(context.Background(), io.MultiReader(&header, generated), io.Discard)
		if (e == nil) != (size <= fetch.MaxSize) {
			t.Fatal(size, e)
		}
	}
}
func TestFetchFrames(t *testing.T) {
	for _, body := range [][]byte{{}, []byte("\x00\xff\r\n"), bytes.Repeat([]byte{255, 0, 13, 10}, 400000)} {
		var out bytes.Buffer
		h, e := Receive(context.Background(), bytes.NewReader(frame(body)), &out)
		if e != nil || h != sha256.Sum256(body) || !bytes.Equal(out.Bytes(), body) {
			t.Fatal("valid frame rejected", e)
		}
	}
	valid := frame([]byte("secret marker"))
	badHash := append([]byte{}, valid...)
	badHash[len(badHash)-1] ^= 1
	oversized := make([]byte, 12)
	copy(oversized, "WTF1")
	binary.LittleEndian.PutUint64(oversized[4:], uint64(fetch.MaxSize+1))
	for _, data := range [][]byte{nil, valid[:3], valid[:len(valid)-1], append(valid, 0), badHash, oversized, bytes.Repeat([]byte{'x'}, 12)} {
		if _, e := Receive(context.Background(), bytes.NewReader(data), io.Discard); e == nil {
			t.Fatal("invalid frame accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Receive(ctx, bytes.NewReader(valid), io.Discard); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e := Receive(context.Background(), bytes.NewReader(valid), failingWriter{}); e == nil {
		t.Fatal("write error ignored")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestFetchWrapperLimits(t *testing.T) {
	for _, target := range []string{"windows", "linux"} {
		b := FetchBackend(target)
		if e := b.Check(); e != nil {
			t.Fatalf("%s: %v", target, e)
		}
		t.Logf("%s wrapper length=%d", target, len(b.LaunchLine))
	}
}

type fetchMemoryStore struct {
	bytes.Buffer
	changed                 bool
	commitError, closeError error
	closed                  bool
}

func (s *fetchMemoryStore) Commit(context.Context, [32]byte) (bool, error) {
	return s.changed, s.commitError
}
func (s *fetchMemoryStore) Close() error { s.closed = true; return s.closeError }

type fetchMemorySession struct {
	code      int
	interrupt bool
}

func (fetchMemorySession) Close() error { return nil }
func (fetchMemorySession) Run(context.Context, transport.Request) (transport.Response, error) {
	panic("ordinary capture used")
}
func (s fetchMemorySession) Stream(ctx context.Context, r transport.Request, c func(io.Reader) error) (transport.Response, error) {
	if s.interrupt {
		return transport.Response{}, errors.New("PRIVATE content and paths")
	}
	if e := c(bytes.NewReader(frame([]byte("PRIVATE")))); e != nil {
		return transport.Response{}, e
	}
	return transport.Response{Communication: transport.CommunicationComplete, ExitCode: &s.code}, nil
}
func TestFetchCommitAndCleanupResults(t *testing.T) {
	for _, mode := range []string{"remote-failure", "interrupted", "cleanup-after-commit", "commit-failure"} {
		t.Run(mode, func(t *testing.T) {
			store := &fetchMemoryStore{changed: true}
			session := fetchMemorySession{}
			switch mode {
			case "remote-failure":
				session.code = 1
			case "interrupted":
				session.interrupt = true
			case "cleanup-after-commit":
				store.closeError = errors.New("PRIVATE")
			case "commit-failure":
				store.changed = false
				store.commitError = errors.New("PRIVATE")
			}
			row := runFetch(context.Background(), Prepared{}, session, "fixed", func(context.Context, string) (fetchStore, error) { return store, nil })
			if row.Status != result.Failed || row.Error == nil || row.Changed == nil || *row.Changed != (mode == "cleanup-after-commit") || !store.closed {
				t.Fatal(row)
			}
		})
	}
}
