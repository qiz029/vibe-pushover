package main

import (
	"context"
	"fmt"
	"os"

	"github.com/qiz029/vibe-pushover/internal/command"
)

var version = "dev"

func main() {
	app := command.New(command.Options{Version: version})
	if err := app.Run(context.Background(), os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
