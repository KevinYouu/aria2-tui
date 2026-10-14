# TUI architecture

aria2-tui runs **one long lived `tea.Program`**. The paste view and the
download list are sub models of a single root model, not separate programs.
This document records the resulting message flow and key handling.

## Why one program

The obvious alternative - one `tea.NewProgram` per screen - loses the state
that has to survive a screen switch: the selected GID, the last snapshot, the
connection count and the message line. Screens would also each need their own
polling loop. A single root model owns that state, keeps exactly one poll timer
alive and makes a future settings view a new sub model rather than a new
program.

```
cmd/aria2-tui/commands/tui.go
  aria2.Start ──▶ tui.Run(ctx, Deps{Client, Store, Config, InitialURIs, DaemonDone})
                    │
                    ▼
                 App (app.go) ── viewList ─▶ listModel (listview.go)
                    │            viewAdd  ─▶ addModel  (addview.go)
                    └── tea.Program
  daemon.Stop ◀──── defer
```

## Views

| View | State it owns | Rendered by |
|---|---|---|
| `viewList` | `cursorGID`, `offset` (first visible row) | `listModel.view` |
| `viewAdd` | the `textarea` and its cursor | `addModel.view` |

Shared state lives on `App`: the snapshot, the message line, the connection
count, the terminal size and the quit confirmation. `App.View` renders the
active view; `App.Update` routes key presses to it.

Startup landing spot:

* links on the command line → list view, `addUri` issued from `Init`
* empty download list → paste view (after the first snapshot)
* downloads already present → list view

## Message flow

| Message | Produced by | Handling |
|---|---|---|
| `tickMsg` | `tea.Tick(snapshotInterval)` | returns `fetchSnapshotCmd` |
| `snapshotMsg` | the polling command | stores the snapshot, re-arms the tick, clears a stale error |
| `actionMsg` | pause / remove / add commands | updates the message line |
| `concurrencyMsg` | the `+`/`-` command | applies the new value or rolls back |
| `daemonExitMsg` | a command that waits on `Daemon.Done()` | reports that aria2c is gone |
| `tea.KeyPressMsg` | the framework | routed to the active view |
| `tea.PasteMsg` | bracketed paste | forwarded to the paste view |

Polling never piles up: a tick produces exactly one request, and the next tick
is armed only after that request has been answered. All RPCs run inside
`tea.Cmd` closures, so `Update` and `View` perform no I/O and stay easy to test.
A failed snapshot keeps the previous list on screen and only updates the message
line, so a killed daemon does not blank the UI.

## Keys

| View | Key | Action |
|---|---|---|
| paste | `ctrl+s` | parse the text area and submit the links |
| paste | `esc` | back to the list |
| paste | `ctrl+c` | quit |
| list | `↑`/`k`, `↓`/`j` | move the selection (it follows the download's GID) |
| list | `pgup` / `pgdown` | move by one visible window |
| list | `p` / `space` | pause ⇄ resume |
| list | `d` | remove; a finished download only drops its result |
| list | `+` / `-` | change the connection count (1..16) |
| list | `a` | open the paste view |
| list | `q` / `ctrl+c` | quit, with an inline `[y/N]` confirmation while downloads are running |
| confirmation | `y` | quit; any other key cancels |

`ctrl+s`, `esc` and `q` are free: the bubbles v2 `textarea` already consumes
`ctrl+d`, `ctrl+e`, `ctrl+k`, `ctrl+u` and `ctrl+v`.

The help bar is generated from the same `key.Binding` values that are matched,
so it cannot advertise a key that is not handled.

## Layout

`layout.go` owns the width arithmetic: `rowColumnsFor` drops columns
(connections → ETA → progress bar) until the name column reaches
`nameMinWidth`, and `listGeometryFor` decides how many rows fit and whether the
message line and help bar are rendered. Every rendered line is truncated to the
terminal width, and the row is cut as a whole when even the minimum layout does
not fit, so the renderer never wraps a row in the middle.

The progress bar is `bubbles/v2/progress` rendered statelessly with
`ViewAs(ratio)` and `█`/`░` fill characters, which stay readable on old Windows
consoles. Emoji are avoided for the same reason.

## Styling and theming

Styles are built on demand in `styles.go` instead of being package level
variables, because `theme.ApplyMode` swaps the palette at runtime and a style
captures its colors when it is built.
