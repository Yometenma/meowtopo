package main

import (
	"log/slog"
	"os"

	"github.com/moetopo/moetopo/internal/app"
)

var version = "dev"

func main() {
	if err := app.Run(version); err != nil {
		slog.Error("MoeTopo stopped", "error", err)
		os.Exit(1)
	}
}
