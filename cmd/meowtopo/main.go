package main

import (
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	"github.com/Yometenma/meowtopo/internal/app"
)

var version = "dev"

func main() {
	if err := app.Run(resolvedVersion()); err != nil {
		slog.Error("MeowTopo stopped", "error", err)
		os.Exit(1)
	}
}

func resolvedVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return "dev-" + setting.Value[:min(8, len(setting.Value))]
		}
	}
	if strings.TrimSpace(info.Main.Version) != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
