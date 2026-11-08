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
	legacy := r.Backends == nil && (r.Wrappers != nil || r.Decode != nil || r.DecodeResult != nil)
	if r.Backends == nil {
		r.Backends = module.DefaultBackends()
		// Legacy injected runtimes are explicitly Windows-only.
		if legacy {
			r.Backends = module.Backends{}
			for kind, line := range r.Wrappers {
				r.Backends[module.Key{OS: "windows", Module: kind}] = module.Backend{
					Prepare: module.Prepare, Command: line, LaunchLine: line,
					ValidateLaunch: module.ValidateWrapper,
					// These backends perform preflight only. Execution uses the
					// legacy runtime so decoders receive the complete TaskPlan.
					Decode: func(transport.Response, bool) result.TaskResult {
						return result.TaskResult{Status: result.Failed, Error: result.PublicError("internal_error")}
					},
				}
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
		if legacy {
			for hi := range o.Plan.Hosts {
				for ti := range o.Plan.Hosts[hi].Tasks {
					o.Plan.Hosts[hi].Tasks[ti].Backend = nil
				}
			}
		}
		tasks, err := r.Execute(ctx, o.Plan, o.Forks, o.Timeout, o.IncludeOutput)
		return result.Finish(ctx, stdout, stderr, tasks, o.Format, o.IncludeOutput, err)
	}, lines, r.Backends)
}
