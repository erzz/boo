// Command boo is the project launcher CLI for Ghostty.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/erzz/boo/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRoot()
	// cobra prints the error itself ("Error: ...") because SilenceErrors is
	// false by default. We just exit non-zero on failure.
	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
