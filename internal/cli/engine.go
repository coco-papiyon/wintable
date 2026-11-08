package cli

import (
	"context"
	"io"

	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

// RunEngine validates every input and fixed wrapper before allowing connections.
// A nil connector selects mandatory-host-key SSH. An unspecified module runtime
// selects the built-in command and file_content distributions.
func RunEngine(ctx context.Context, args []string, stdout, stderr io.Writer, r engine.Runtime) int {
	if r.Backends == nil {
		r.Backends = module.WindowsBackends()
		// Legacy injected runtimes are explicitly Windows-only.
		if r.Wrappers != nil || r.Decode != nil || r.DecodeResult != nil {
			r.Backends = module.Backends{}
			for kind, line := range r.Wrappers {
				k := kind
				r.Backends[module.Key{OS: "windows", Module: k}] = module.Backend{Prepare: module.Prepare, Command: line, LaunchLine: line, ValidateLaunch: module.ValidateWrapper, Decode: func(response transport.Response, output bool) result.TaskResult {
					t := engine.TaskPlan{Module: k}
					if r.DecodeResult != nil {
						return r.DecodeResult(t, response, output)
					}
					if r.Decode == nil {
						return result.TaskResult{Status: result.Failed, Error: result.PublicError("internal_error")}
					}
					status, changed, err := r.Decode(t, response)
					return result.TaskResult{Status: status, Changed: changed, Error: err, ExitCode: response.ExitCode}
				}}
			}
		}
	}
	if r.Connector == nil {
		r.Connector = transport.SSHConnector{}
	}
	lines := make([]string, 0, len(r.Wrappers))
	for _, line := range r.Wrappers {
		lines = append(lines, line)
	}
	return runWithBackends(ctx, args, stdout, stderr, func(ctx context.Context, o Options) int {
		tasks, err := r.Execute(ctx, o.Plan, o.Forks, o.Timeout, o.IncludeOutput)
		return result.Finish(ctx, stdout, stderr, tasks, o.Format, o.IncludeOutput, err)
	}, lines, r.Backends)
}
