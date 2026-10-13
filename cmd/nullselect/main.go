package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/null-select/cli/internal/command"
)

var (
	version = "dev"
	commit  = "none"
	builtAt = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := command.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, command.Build{
		Version: version,
		Commit:  commit,
		BuiltAt: builtAt,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "nullselect:", err)
		os.Exit(1)
	}
}
