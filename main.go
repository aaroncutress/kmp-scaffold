// Command kmp-scaffold creates and extends Kotlin Multiplatform projects.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/aaroncutress/kmp-scaffold/internal/cli"
)

func main() {
	// Ctrl+C during a network call should stop it, not leave the terminal in a
	// half-drawn state.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Run(ctx, os.Args[1:]))
}
