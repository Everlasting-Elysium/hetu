// Command hetu is the single binary entrypoint. It wires OS signals into a
// cancellable context and delegates to the cli command tree.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Everlasting-Elysium/hetu/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.Execute(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "hetu:", err)
		os.Exit(1)
	}
}
