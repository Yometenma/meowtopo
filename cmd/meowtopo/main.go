package main

import (
	"log/slog"
	"os"

	"github.com/Yometenma/meowtopo/internal/app"
)

var version = "dev"

func main() {
	if err := app.Run(version); err != nil {
		slog.Error("MeowTopo stopped", "error", err)
		os.Exit(1)
	}
}
