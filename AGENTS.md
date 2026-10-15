# AGENTS.md

## Overview

aria2-tui is a terminal download manager in Go 1.27: a Bubble Tea (`charm.land/bubbletea/v2`) TUI that starts and drives its own `aria2c --enable-rpc` child over JSON-RPC. CLI via cobra, settings in `~/.aria2-tui.json`, no database or web layer. Docs and user-facing strings are English.

## Commands

- `make all` — run before finishing. There is no test CI job; `.github/workflows/release.yaml` only runs GoReleaser on tags.
- `go test ./internal/tui -run <Test>` — single test; the Makefile has no per-package target.
- `make run <url>` — build and run with arguments.

## Layout

- `internal/aria2` — daemon lifecycle (`daemon.go`) and the JSON-RPC client; the only package that talks to aria2c.
- `internal/tui` — one long-lived root model in `app.go` plus one file per view; `layout.go` owns width arithmetic, `styles.go` the styles.
- `internal/logs` — stderr warnings/errors for the CLI; there is no log file.
- `docs/features` — design rationale for the daemon lifecycle and the TUI structure.

## Conventions

- Build lipgloss styles on demand in `styles.go`; package-level styles freeze their colors and break the runtime `theme.ApplyMode` swap.
- Avoid emoji and exotic glyphs: the UI must render on old Windows consoles.
- Keep one `tea.Program`. New screens are sub-models of `App`, and all RPC runs inside `tea.Cmd`s so `Update`/`View` stay I/O-free.

## Pitfalls

- `--theme` is pre-parsed in `cmd/aria2-tui/main.go` (`resolveTheme`) so help output is already themed; reading the cobra flag in `root.go` instead would undo that.
- Daemon shutdown must pass a fresh `context.WithTimeout(context.Background(), …)`: the signal context is already cancelled on `SIGINT` and would skip the graceful RPC shutdown.
- In `internal/aria2/daemon.go`, do not add `--no-conf` and do not drop `--daemon=false`, `--stop-with-process`, `--continue=true` or `--seed-time=0` — the user's `aria2.conf` stays loaded and only the explicit CLI flags protect the session from it. `stdin` is nil so aria2c cannot steal keystrokes.
- TUI tests drive the model through the `fakeRPC` harness in `internal/tui/testsupport_test.go` instead of a real daemon; the `internal/aria2` tests that need a real `aria2c` skip themselves when it is not on `PATH`.
