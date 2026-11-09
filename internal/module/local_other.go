//go:build !windows

package module

import (
	"context"
	"github.com/coco-papiyon/wintable/internal/result"
)

func RunLocalCommand(context.Context, Prepared, bool) result.TaskResult {
	return result.TaskResult{Status: result.Failed, Error: result.PublicError("internal_error")}
}
