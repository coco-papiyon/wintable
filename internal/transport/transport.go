// Package transport owns SSH connection, host-key validation and session lifetime.
// It does not import the engine, CLI, modules or result renderer.
package transport

import "context"

// Host is validated connection information. Never serialize it into task results.
type Host struct {
	Name, Address, User, PrivateKeyFile, KnownHostsFile string
	Port                                                uint16
}

// Request separates the fixed wrapper launch line from BOM-free UTF-8 JSON.
// Task values must only occur in Stdin, never in Command.
type Request struct {
	Command string
	Stdin   []byte
	// StdoutLimit is bounded by WrapperResponseLimit; zero uses CaptureLimit.
	// Module JSON contains two base64-encoded 1MiB streams plus framing.
	StdoutLimit int
}

type CommunicationState string

const (
	CommunicationUnknown     CommunicationState = "unknown"
	CommunicationComplete    CommunicationState = "complete"
	CommunicationInterrupted CommunicationState = "interrupted"
)

// Response separates transport completion from the remote process result.
// ExitCode nil means unavailable; nonzero is a process failure, not a Run error.
// Raw bytes preserve the original encoding. Capture is bounded per stream;
// readers must continue draining discarded bytes after reaching the limit.
type Response struct {
	Communication                    CommunicationState
	ExitCode                         *int
	Stdout, Stderr                   []byte
	StdoutTruncated, StderrTruncated bool
}

type Connector interface {
	// Connect and Run may return a result.Error (also wrapped) to supply an
	// explicit public classification. The engine sanitizes its Code through
	// result.PublicError; unknown codes become internal_error. Unclassified
	// errors use operation-specific connection defaults; raw text is never public.
	// Connect must honor ctx and release partial resources on failure. A returned
	// Session must outlive the connect context; the caller cancels it immediately.
	Connect(context.Context, Host) (Session, error)
}

// Session implementations must include send, stdin EOF and concurrent output
// reads in ctx cancellation/deadlines, release resources on cancellation, and
// never retry a request. Run errors describe transport/timeout/cancellation;
// partial Response data may still be available. Closing the session does not
// guarantee termination of a remote child process. Callers always Close after use.
// Close must be safe concurrently with Run and promptly unblock its I/O. The
// engine invokes Close exactly once, including cancellation and deadline paths.
type Session interface {
	Run(context.Context, Request) (Response, error)
	Close() error
}
