package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/coco-papiyon/wintable/internal/cli"
	"github.com/coco-papiyon/wintable/internal/engine"
)

func main() { os.Exit(run()) }

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return cli.RunEngine(ctx, os.Args[1:], os.Stdout, os.Stderr, engine.Runtime{})
}
