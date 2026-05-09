// Command boo is the project launcher CLI for Ghostty.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/erzz/boo/internal/cli"
)

// Build-time variables injected via -ldflags by GoReleaser.
// Defaults apply when building with plain `go build` or `make build`.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.NewRoot(version, commit, date)
	// cobra prints the error itself ("Error: ...") because SilenceErrors is
	// false by default. We just exit non-zero on failure.
	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
