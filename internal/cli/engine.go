package cli

import (
	"context"
	"io"

	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/result"
	"github.com/coco-papiyon/wintable/internal/transport"
)

// RunEngine validates every input and fixed wrapper before allowing connections.
// A nil connector selects mandatory-host-key SSH. Wrappers and Decode must be
// supplied by the module distribution; the production entry awaits those modules.
func RunEngine(ctx context.Context, args []string, stdout, stderr io.Writer, r engine.Runtime) int {
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
