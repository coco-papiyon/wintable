package module

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/coco-papiyon/wintable/internal/fetch"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

//go:embed fetch.cs
var fetchSource string

//go:embed fetch_linux.py
var fetchLinuxSource string

func FetchWrapper(target string) string {
	if target == "linux" {
		return "/usr/bin/python3 -I -c '" + strings.ReplaceAll(fetchLinuxSource, "'", "'\"'\"'") + "'"
	}
	// ASCII bootstrap avoids the UTF-16/base64 expansion of EncodedCommand. All
	// characters are fixed distribution code; user values remain in stdin JSON.
	var b bytes.Buffer
	z, _ := flate.NewWriter(&b, flate.BestCompression)
	_, _ = z.Write([]byte(receiverSource + fetchSource))
	_ = z.Close()
	script := `$ErrorActionPreference=1;$ProgressPreference=0;try{$m=[IO.MemoryStream]::new([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString(b.Bytes()) + `'));$z=[IO.Compression.DeflateStream]::new($m,[IO.Compression.CompressionMode]::Decompress);Add-Type ([IO.StreamReader]::new($z).ReadToEnd());[Receiver]::Run();exit [Environment]::ExitCode}catch{exit 1}`
	return `powershell.exe -NoProfile -NonInteractive -Command "` + script + `"`
}

func FetchBackend(target string) Backend {
	line := FetchWrapper(target)
	launch := line
	validate := ValidateLinuxWrapper
	if target == "windows" {
		launch = WrapperLaunchLine(line)
		validate = ValidateWrapper
	}
	return Backend{Command: line, LaunchLine: launch, ValidateLaunch: validate,
		Prepare: func(host string, task int, kind string, args map[string]any) (Prepared, error) {
			src, ok := args["src"].(string)
			dest, other := args["dest"].(string)
			if kind != "fetch" || len(args) != 2 || !ok || !other {
				return Prepared{}, fmt.Errorf("input_error: invalid fetch arguments")
			}
			path, e := fetch.Destination(host, target, src, dest)
			if e != nil {
				return Prepared{}, e
			}
			p, e := Serialize(host, task, "fetch", map[string]any{"src": src})
			p.FetchDestination = path
			return p, e
		}, Decode: func(transport.Response, bool) result.TaskResult {
			return result.TaskResult{Status: result.Failed, Error: result.PublicError("internal_error")}
		}}
}

// Receive validates framing, bounded length, SHA-256 and EOF without retaining
// file content. Remote exit success is checked by RunFetch before committing.
func Receive(ctx context.Context, r io.Reader, w io.Writer) ([32]byte, error) {
	var digest [32]byte
	if ctx.Err() != nil {
		return digest, ctx.Err()
	}
	var header [12]byte
	if _, e := io.ReadFull(r, header[:]); e != nil {
		return digest, e
	}
	size := binary.LittleEndian.Uint64(header[4:])
	if string(header[:4]) != "WTF1" || size > uint64(fetch.MaxSize) {
		return digest, fmt.Errorf("invalid transfer header")
	}
	h := sha256.New()
	buf := make([]byte, 65536)
	remaining := int64(size)
	for remaining > 0 {
		if ctx.Err() != nil {
			return digest, ctx.Err()
		}
		n := int64(len(buf))
		if n > remaining {
			n = remaining
		}
		if _, e := io.ReadFull(r, buf[:n]); e != nil {
			return digest, e
		}
		if count, e := w.Write(buf[:n]); e != nil || int64(count) != n {
			return digest, fmt.Errorf("local write failed")
		}
		h.Write(buf[:n])
		remaining -= n
	}
	copy(digest[:], h.Sum(nil))
	var trailer [32]byte
	if _, e := io.ReadFull(r, trailer[:]); e != nil {
		return digest, e
	}
	if trailer != digest {
		return digest, fmt.Errorf("hash mismatch")
	}
	var extra [1]byte
	if n, e := r.Read(extra[:]); n != 0 || e != io.EOF {
		return digest, fmt.Errorf("missing EOF")
	}
	return digest, nil
}

func RunFetch(ctx context.Context, p Prepared, s transport.Session, command string) (row result.TaskResult) {
	return runFetch(ctx, p, s, command, func(ctx context.Context, path string) (fetchStore, error) { return fetch.Begin(ctx, path) })
}

type fetchStore interface {
	io.Writer
	Commit(context.Context, [32]byte) (bool, error)
	Close() error
}

func runFetch(ctx context.Context, p Prepared, s transport.Session, command string, begin func(context.Context, string) (fetchStore, error)) (row result.TaskResult) {
	changed := false
	row = result.TaskResult{Status: result.Failed, Changed: &changed, Error: result.PublicError("task_failed")}
	// Preserve local certainty on failures: the remote sender has no local side effect.
	defer func() {
		if ctx.Err() != nil {
			code := "cancelled"
			if ctx.Err() == context.DeadlineExceeded {
				code = "deadline_exceeded"
			}
			row.Status = result.Failed
			row.Error = result.PublicError(code)
		}
	}()
	stream, ok := s.(transport.StreamSession)
	if !ok {
		row.Error = result.PublicError("internal_error")
		return
	}
	store, e := begin(ctx, p.FetchDestination)
	if e != nil {
		return
	}
	defer func() {
		if e := store.Close(); e != nil {
			row.Status = result.Failed
			row.Error = result.PublicError("task_failed")
		}
	}()
	var hash [32]byte
	response, e := stream.Stream(ctx, transport.Request{Command: command, Stdin: p.JSON()}, func(r io.Reader) error { var e error; hash, e = Receive(ctx, r, store); return e })
	if e != nil {
		row.Error = result.PublicError("connection_lost")
		if classified, ok := e.(*result.Error); ok {
			row.Error = result.PublicError(classified.Code)
		}
		return
	}
	if response.Communication != transport.CommunicationComplete || response.ExitCode == nil || *response.ExitCode != 0 {
		return
	}
	changed, e = store.Commit(ctx, hash)
	if e != nil {
		return
	}
	row.Status = result.OK
	if changed {
		row.Status = result.Changed
	}
	row.Error = nil
	return
}
