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
// selects the built-in command distribution (file_content awaits Issue #10).
func RunEngine(ctx context.Context, args []string, stdout, stderr io.Writer, r engine.Runtime) int {
	if r.Wrappers == nil && r.Decode == nil && r.DecodeResult == nil {
		if err := module.ValidateCommandWrapper(); err != nil {
			return RunWithWrappers(ctx, args, stdout, stderr, nil, []string{module.WrapperLaunchLine(module.CommandWrapper())})
		}
		r.Wrappers = map[string]string{"command": module.CommandWrapper()}
		r.DecodeResult = func(_ engine.TaskPlan, response transport.Response, output bool) result.TaskResult {
			return module.DecodeCommand(response, output)
		}
	}
	if r.Connector == nil {
		r.Connector = transport.SSHConnector{}
	}
	lines := make([]string, 0, len(r.Wrappers))
	for _, line := range r.Wrappers {
		lines = append(lines, line)
	}
	return RunWithWrappers(ctx, args, stdout, stderr, func(ctx context.Context, o Options) int {
		tasks, err := r.Execute(ctx, o.Plan, o.Forks, o.Timeout, o.IncludeOutput)
		return result.Finish(ctx, stdout, stderr, tasks, o.Format, o.IncludeOutput, err)
	}, lines)
}
