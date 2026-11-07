// Package cli validates command-line options before invoking the execution engine.
package cli

import (
	"context"
	"flag"
	"fmt"
	"github.com/coco-papiyon/wintable/internal/config"
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
PowerShell 5.1+, independent JSON v1; command and file_content modules.
Unsupported: Ansible YAML/Jinja/collections, roles/handlers/facts/become/vault,
dynamic inventory, check mode, retries, rollback, WinRM, arm64 and Linux targets.
Output bodies may contain secrets from arbitrary commands. Automatic masking
is not guaranteed; enable --include-output only when appropriate.

Input files are strictly validated before execution. No connections are made yet.
Valid run options report execution_not_implemented and exit 3 until the engine
is integrated. JSON result rendering is also deferred to the result stage.
Exit codes: 0 success/help, 1 target failure, 2 input, 3 internal, 130 cancelled.
`

// Run parses without network access. A nil executor is the explicit foundation stub.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, execute Executor) int {
	showHelp := func() int {
		if _, err := io.WriteString(stdout, help); err != nil {
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
	if execute == nil {
		fmt.Fprintln(stderr, "execution_not_implemented: engine integration pending")
		return 3
	}
	return execute(ctx, o)
}
