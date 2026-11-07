package cli

import (
	"context"
	"io"

	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/result"
)

// RunEngine validates every input and fixed wrapper before allowing connections.
// Production wiring awaits the SSH connector and module implementations.
func RunEngine(ctx context.Context, args []string, stdout, stderr io.Writer, r engine.Runtime) int {
	lines := make([]string, 0, len(r.Wrappers))
	for _, line := range r.Wrappers {
		lines = append(lines, line)
	}
	return RunWithWrappers(ctx, args, stdout, stderr, func(ctx context.Context, o Options) int {
		tasks, err := r.Execute(ctx, o.Plan, o.Forks, o.Timeout, o.IncludeOutput)
		return result.Finish(ctx, stdout, stderr, tasks, o.Format, o.IncludeOutput, err)
	}, lines)
}
