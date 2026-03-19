# AGENTS.md

Guidance for coding agents working in `gh-actions-watcher`.

## Purpose

This repository ships `gha-watch`, a fail-fast GitHub Actions workflow-run watcher implemented in Go and distributed through npm.

## Architecture

- `cmd/gha-watch/main.go`: process entrypoint, error handling, exits non-zero on failure.
- `internal/app/app.go`: command parser, GitHub API client, watch loop.
- `internal/app/app_test.go`: unit tests for help/fail-fast/success paths.
- `bin/gha-watch.js`: npm shim that invokes packaged native binary.
- `scripts/postinstall.js`: downloads release binary on install, falls back to local `go build`.
- `.github/workflows/release.yml`: tag-driven release pipeline.

## Local commands

Use `make` targets:

- `make fmt`
- `make test`
- `make vet`
- `make lint`
- `make check`
- `make build`
- `make build-all`
- `make install-local`

Direct commands:

- `go test ./...`
- `go vet ./...`
- `npm run lint`

## Guardrails

1. Keep binary naming contract aligned across workflow and postinstall:
- release asset: `gha-watch_<goos>_<goarch>[.exe]`
- npm installed path: `bin/gha-watch-bin` (or `bin/gha-watch.exe` on Windows)

2. If `CLI_BINARY` changes, update all of:
- `cmd/<binary>`
- `package.json` `bin` + `config.cliBinaryName`
- `Makefile` `BIN_NAME`
- `.github/workflows/release.yml`
- `scripts/postinstall.js`

3. Keep watch behavior fail-fast:
- fail on first job/step with bad conclusion
- print transitions only when status/conclusion changes
