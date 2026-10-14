# aria2c daemon ownership

aria2-tui does not talk to a shared or remote aria2 instance. Every session
starts its own private `aria2c --enable-rpc` child and shuts it down on exit.
This document records why, and what that means for the process lifecycle.

## Why a private daemon

| Requirement | Consequence |
|---|---|
| "quitting the app must stop the daemon" | The daemon belongs to the session, so no orphan can survive a crash |
| no configuration UI for RPC auth | The secret is generated per session and never written to disk |
| nothing may listen on the network | The daemon binds `127.0.0.1` only, on a port picked by the kernel |
| Windows has no signals | Shutdown cannot rely on `SIGTERM` |

A long lived daemon plus an `attach` mode would need a session file, a
discovery mechanism and an auth UI. That is explicitly out of scope for the
MVP; the cost is that closing the app interrupts running downloads (partial
data stays on disk and resumes when the same link is added again).

## Startup

```
freePort()            net.Listen("tcp", "127.0.0.1:0"), then close: the port is
                      released for aria2 to bind. Another process could grab it
                      in that window, in which case the readiness probe fails.
randomSecret()        crypto/rand, 32 bytes hex; passed as --rpc-secret only.
exec.Command(aria2c)  stdin=nil, stdout/stderr=<ringBuffer>
waitReady()           poll aria2.getVersion every 50ms, up to 5s
```

`stdin` is set to `nil` on purpose: an aria2c that inherits the terminal can
steal keystrokes from the TUI. `stdout`/`stderr` go to a locked ring buffer of
the last 8 KiB, which keeps subprocess output out of the alternate screen
while still making a startup failure diagnosable (the buffer tail is appended
to the error).

The readiness probe doubles as the version query: the version shown in the
status bar comes from `aria2.getVersion`, not from parsing `aria2c --version`.

## Command line

```
--enable-rpc=true
--rpc-listen-all=false
--rpc-listen-port=<free port>
--rpc-secret=<random>
--rpc-allow-origin-all=false
--daemon=false
--stop-with-process=<pid of aria2-tui>
--dir=<absolute download dir>
--max-concurrent-downloads=<n>
--continue=true
--console-log-level=warn
--seed-time=0
```

* `--no-conf` is **not** passed: the user's `aria2.conf` keeps providing proxy,
  tracker and header settings. Every option above is also a CLI flag, and aria2
  gives the command line priority over the config file, so a stray
  `daemon=true` or `continue=false` in `aria2.conf` cannot break a session.
* `--daemon=false` matters: a detached process would break both pid tracking
  and `--stop-with-process`.
* `--stop-with-process=<pid>` is the last resort for a terminal that is closed
  without a signal (`SIGHUP`, `SIGKILL`): aria2c exits by itself within about a
  second once the parent is gone.
* `--console-log-level=warn` suppresses the notice level progress summary (which
  would flood the ring buffer) while keeping warnings and fatal errors.
* `--continue=true`: aria2 defaults this to `false`, which would restart a
  partially downloaded file from byte 0.
* `--seed-time=0`: a download manager must not keep seeding after the download.

## Shutdown

```
Stop(ctx):
  1. aria2.shutdown over RPC, waiting at most 5s
  2. kill the process as a fallback
  3. wait for the reaped process
```

`Stop` must be called with a context that is *not* the signal cancelled one:
on `SIGINT` the parent context is already done and a graceful RPC shutdown
would be skipped. The caller therefore passes a fresh
`context.WithTimeout(context.Background(), 10s)`, so the RPC shutdown and the
kill fallback both still run. `aria2.shutdown` closes the connection while
answering, so a transport level error is treated as success; only an RPC fault
is reported.

## Failure modes

| Failure | Behaviour |
|---|---|
| aria2c not installed | English error with per-platform install hints, exit code 1, no daemon started |
| aria2c exits during startup | error carries the exit code and the captured stderr tail |
| aria2c killed while the TUI runs | the exit is reported in the message line and the list keeps rendering with the last snapshot |
| terminal closed without a signal | `--stop-with-process` reaps aria2c |
| leaked port between `freePort` and bind | readiness probe fails with a clear error instead of hanging |
