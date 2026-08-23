# MeowTopo Development Guide

This file applies to the entire repository.

## Project identity

- Product name: `MeowTopo`; Chinese name: `喵拓`.
- Repository: `https://github.com/Yometenma/meowtopo`.
- Go module: `github.com/Yometenma/meowtopo`.
- Main command: `./cmd/meowtopo`.
- Docker image: `meowtopo/meowtopo`.
- Environment variable prefix: `MEOWTOPO_`.
- SQLite database filename: `meowtopo.db`.
- Do not reintroduce the former names `MoeTopo`, `moetopo`, or `MOETOPO`.

## Product boundaries

MeowTopo is a lightweight, local-first home and Homelab network topology monitor. Keep the application as a single Go service with an embedded frontend and local SQLite storage unless a task explicitly changes that architecture.

- Do not add telemetry, advertising, cloud synchronization, or required cloud services.
- The normal local default must remain bound to `127.0.0.1`; do not expose the application publicly by default.
- Network discovery must remain limited to user-confirmed RFC 1918 private ranges and preserve the documented scan-size and concurrency limits.
- Probes must not become vulnerability scans, credential attempts, or general-purpose HTTP crawling.
- Automatic discovery must never overwrite user-provided device names, types, notes, topology relationships, or canvas coordinates.
- Do not claim inferred topology is physical topology. Preserve confidence/source distinctions for inferred and user-defined relationships.
- Keep runtime frontend dependencies local and embedded; do not introduce a CDN dependency without explicit approval.

## Assets and privacy

- Read `ASSETS_LICENSE.md` before changing visual assets.
- `internal/app/web/assets/topo-chan.png` is the user-provided official Topo-chan visual reference and is not covered by the repository's MIT license.
- Do not generate, replace, redraw, transform, or redistribute official character artwork without explicit user approval.
- Never commit real household network data, including IP addresses, MAC addresses, hostnames, internal domains, serial numbers, logs, backups, or SQLite databases. Tests and examples must use fictional private-network data.

## Working practices

- Read `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, and `ASSETS_LICENSE.md` before substantial changes.
- Inspect `git status` before editing. Preserve all existing user changes and avoid unrelated rewrites.
- Make the smallest coherent change that completes the requested work.
- Keep configuration defaults, documentation, `.env.example`, Compose, and implementation synchronized.
- Add or update tests when changing scanners, topology inference, device identification, backup/restore, security boundaries, or configuration parsing.
- Do not commit generated binaries, runtime databases, logs, backups, or the `data` directory.
- Do not create commits, push branches, or modify remote state unless the user asks.

## Validation

Run the checks relevant to every change, and run the complete set before release-oriented handoff:

```bash
go test ./...
go vet ./...
node --check internal/app/web/app.js
go build -trimpath -ldflags="-s -w" -o meowtopo.exe ./cmd/meowtopo
docker compose config
```

When Docker is available, also validate the image and container health:

```bash
docker build -t meowtopo/meowtopo:local .
docker compose up -d
```

Verify `/api/version`, `/api/health`, the home page, and creation of `meowtopo.db`. If a required tool or Docker engine is unavailable, report that as an environment limitation; do not describe an unexecuted check as passing.

Before finishing, search the whole working tree (excluding `.git`, generated binaries, and runtime data) for the former project name and confirm the worktree contains only intended changes.
