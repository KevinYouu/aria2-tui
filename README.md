# aria2-tui

A terminal download manager built on [aria2](https://aria2.github.io/). Paste
links, watch progress, pause, resume and remove downloads without leaving the
terminal.

```
aria2-tui ───────────────────────────────────────────────────────────────────
❯  ubuntu-24.04.iso        ███████████░░░░░   71.3%   8.2 MiB/s   ETA 00:41   active    8 conn
   debian-13.iso           ████████░░░░░░░░   47.8%   5.1 MiB/s   ETA 01:12   active    4 conn
5 connections  ↓ 13.3 MiB/s  ↑ 0 B/s  2 active · 0 waiting · 1 done  aria2c 1.37.0
[↑/↓] navigate · [p/space] pause · resume · [d] remove · [+/-] connections · [a] add links · [q] quit
```

## Requirements

`aria2c` must be installed and on `PATH` (or set `aria2c_path` in the config):

| Platform | Command |
|---|---|
| macOS | `brew install aria2` |
| Debian/Ubuntu | `sudo apt install aria2` |
| Fedora | `sudo dnf install aria2` |
| Windows | `scoop install aria2` or `winget install aria2.aria2` |

## Install

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/KevinYouu/aria2-tui/main/install.sh | bash
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/KevinYouu/aria2-tui/main/install.ps1 | iex
```

Or build from source:

```sh
go install github.com/KevinYouu/aria2-tui/cmd/aria2-tui@latest
```

## Usage

```sh
aria2-tui                            # start and paste links
aria2-tui https://example.com/f.iso  # start and download immediately
aria2-tui version                    # print the version
```

Links are validated before anything starts, so an unusable argument fails with
an English error and exit code 1 instead of launching a daemon.

### Keys

| View | Key | Action |
|---|---|---|
| paste | `ctrl+s` | submit the pasted links |
| paste | `esc` | back to the list |
| list | `↑`/`k`, `↓`/`j` | move the selection |
| list | `pgup`, `pgdown` | move by one screen |
| list | `p`, `space` | pause ⇄ resume |
| list | `d` | remove the download |
| list | `+`, `-` | change how many downloads run at once |
| list | `a` | add more links |
| list | `q`, `ctrl+c` | quit (with a `[y/N]` prompt while downloads run) |

## How it works

Each session starts a private `aria2c --enable-rpc` child on `127.0.0.1` with a
random port and secret, drives it over JSON-RPC and stops it on exit - by RPC
shutdown, then by killing the process if it does not go away. The user's
`aria2.conf` is still loaded for proxy and tracker settings, but every option
this tool depends on is passed explicitly on the command line.

Details: [docs/features/aria2-daemon.md](docs/features/aria2-daemon.md) and
[docs/features/tui-architecture.md](docs/features/tui-architecture.md).

## Configuration

`~/.aria2-tui.json` is created on demand:

```json
{
  "download_dir": "/Users/you/Downloads",
  "max_concurrent_downloads": 5,
  "aria2c_path": "",
  "theme": "auto"
}
```

| Field | Default | Notes |
|---|---|---|
| `download_dir` | `~/Downloads` | `~` is expanded; the directory is created if missing |
| `max_concurrent_downloads` | `5` | `1`–`16`; `+`/`-` in the list applies it live and saves it |
| `aria2c_path` | `""` | empty means "look up `aria2c` in `PATH`" |
| `theme` | `auto` | `auto`, `dark` or `light`; `--theme` overrides and writes it back |

## Limitations

* The aria2c session ends with the app: running downloads stop and resume the
  next time the same link is added (partial files and `.aria2` control files
  stay on disk, `--continue` is enabled).
* There is no way to hand links to a running instance: the daemon's lifetime is
  the app's lifetime.
* Settings are limited to the four fields above; there is no UI for them yet.
* The UI is English only.
* Old Windows consoles (`conhost`) may render the box drawing characters and
  colors poorly; Windows Terminal is recommended.

## Development

```sh
make all     # vet, fmt, test, build
make test
make run https://example.com/f.iso
```

Cross compilation for all release targets:

```sh
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build ./...
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build ./...
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build ./...
```

## License

GPL-3.0. See [LICENSE](LICENSE).
