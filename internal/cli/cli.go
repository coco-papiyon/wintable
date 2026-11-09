// Package cli validates command-line options before invoking the execution engine.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/config"
	"github.com/coco-papiyon/wintable/internal/engine"
	"github.com/coco-papiyon/wintable/internal/module"
	"github.com/coco-papiyon/wintable/internal/result"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// Options contains CLI values; file contents and host selection are validated downstream.
type Options struct {
	Inventory, Tasks, Limit, VarsFile string
	Forks                             int
	Timeout                           time.Duration
	Format                            string
	IncludeOutput                     bool
	Config                            *config.Bundle
	Plan                              *engine.Plan
}

// Executor receives validated options and the signal-aware parent context.
// It returns a product exit code (0, 1, 2, 3 or 130).
type Executor func(context.Context, Options) int

const help = `Usage: wintable run --inventory FILE --tasks FILE [options]
       wintable help | --help | run --help

Options:
  --inventory FILE     Required static inventory JSON v1 (no default)
  --tasks FILE         Required tasks JSON v1 (no default)
  --limit HOSTS        Comma-separated exact host names (default: selected hosts)
  --forks N            Concurrent hosts, 1..64 (default: 5)
  --timeout DURATION   Positive Go duration per task (default: 60s)
  --format FORMAT      text or json (default: text)
  --vars-file FILE     String dictionary JSON (default: none)
  --include-output     Include stdout/stderr bodies (default: false)

Scope: Windows 11 / Windows Server 2022 or later, amd64 control and target;
SSH public-key authentication, mandatory known_hosts, cmd.exe default shell,
PowerShell 5.1+, independent JSON v1; command, file_content and fetch modules.
Debian 13 amd64 targets require OpenSSH, POSIX /bin/sh and CPython 3.13.
Unsupported: Ansible YAML/Jinja/collections, roles/handlers/facts/privilege escalation/vault,
dynamic inventory, check mode, retries, rollback, WinRM and arm64.
Output bodies may contain secrets from arbitrary commands. Automatic masking
is not guaranteed; enable --include-output only when appropriate.

Input files are strictly validated before execution. No connections are made yet.
Fetch saves binary files on the Windows controller; see docs/fetch-module.md.
Local command: task delegate_to="localhost" runs on the Windows controller.
become accepts false only; no elevation. Absolute .exe + argv required.
Delegation runs once per original host; local-only tasks need no SSH credentials.
See docs/local-command.md and examples/local-{inventory,tasks}.json.
Internal errors exit 3.
Exit codes: 0 success/help, 1 target failure, 2 input, 3 internal, 130 cancelled.
`

// Run parses without network access. A nil executor is the explicit foundation stub.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, execute Executor) int {
	return RunWithWrappers(ctx, args, stdout, stderr, execute, nil)
}

// RunWithWrappers is the integration entry for distribution-provided final
// OpenSSH/cmd launch lines. All are checked before the executor can connect.
func RunWithWrappers(ctx context.Context, args []string, stdout, stderr io.Writer, execute Executor, wrapperLines []string) (code int) {
	return runWithBackends(ctx, args, stdout, stderr, execute, wrapperLines, nil)
}
func runWithBackends(ctx context.Context, args []string, stdout, stderr io.Writer, execute Executor, wrapperLines []string, backends module.Backends) (code int) {
	tracked := &diagnosticWriter{Writer: stderr}
	stderr = tracked
	defer func() {
		extra := 0
		if tracked.failed {
			extra = 3
		}
		code = result.ExitCode(ctx, result.Counts{}, code, extra)
	}()
	showHelp := func() int {
		if err := result.WriteAll(stdout, []byte(help)); err != nil {
			return 3
		}
		return 0
	}
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		return showHelp()
	}
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(stderr, "input_error: expected run or help")
		return 2
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	// Do not echo supplied values: even invalid arguments may contain secrets.
	fs.SetOutput(io.Discard)
	var o Options
	fs.StringVar(&o.Inventory, "inventory", "", "")
	fs.StringVar(&o.Tasks, "tasks", "", "")
	fs.StringVar(&o.Limit, "limit", "", "")
	fs.StringVar(&o.VarsFile, "vars-file", "", "")
	fs.IntVar(&o.Forks, "forks", 5, "")
	fs.DurationVar(&o.Timeout, "timeout", 60*time.Second, "")
	fs.StringVar(&o.Format, "format", "text", "")
	fs.BoolVar(&o.IncludeOutput, "include-output", false, "")
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return showHelp()
		}
		fmt.Fprintln(stderr, "input_error: invalid or unsupported option (see help)")
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(o.Inventory) == "" || strings.TrimSpace(o.Tasks) == "" || o.Forks < 1 || o.Forks > 64 || o.Timeout <= 0 || (o.Format != "text" && o.Format != "json") {
		fmt.Fprintln(stderr, "input_error: required files or option values invalid (see help)")
		return 2
	}
	if ctx.Err() != nil {
		return 130
	}
	for _, path := range []*string{&o.Inventory, &o.Tasks, &o.VarsFile} {
		if *path != "" {
			resolved, err := filepath.Abs(*path)
			if err != nil {
				fmt.Fprintln(stderr, "input_error: cannot resolve CLI file path")
				return 2
			}
			*path = resolved
		}
	}
	bundle, err := config.Load(o.Inventory, o.Tasks, o.VarsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	o.Config = bundle
	limitSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "limit" {
			limitSet = true
		}
	})
	if backends != nil {
		o.Plan, err = engine.BuildWithBackends(bundle, o.Limit, limitSet, backends)
	} else {
		o.Plan, err = engine.Build(bundle, o.Limit, limitSet, wrapperLines...)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		var size *module.SizeError
		if errors.As(err, &size) && size.Internal {
			return 3
		}
		if strings.HasPrefix(err.Error(), "internal_error:") {
			return 3
		}
		return 2
	}
	if execute == nil {
		fmt.Fprintln(stderr, "execution_not_implemented: engine integration pending")
		return 3
	}
	return execute(ctx, o)
}

// ResultExecutor returns credential-free task results and an optional classified
// run failure. Engine integration uses this instead of returning an exit code.
type ResultExecutor func(context.Context, Options) ([]result.TaskResult, error)

func RunResults(ctx context.Context, args []string, stdout, stderr io.Writer, execute ResultExecutor) int {
	if execute == nil {
		return Run(ctx, args, stdout, stderr, nil)
	}
	return Run(ctx, args, stdout, stderr, func(ctx context.Context, o Options) int {
		tasks, err := execute(ctx, o)
		return result.Finish(ctx, stdout, stderr, tasks, o.Format, o.IncludeOutput, err)
	})
}

type diagnosticWriter struct {
	io.Writer
	failed bool
}

func (w *diagnosticWriter) Write(p []byte) (int, error) {
	n, e := w.Writer.Write(p)
	if e != nil || n != len(p) {
		w.failed = true
	}
	return n, e
}
