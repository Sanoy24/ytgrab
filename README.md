# YTGrab

A local YouTube downloader with a clean browser interface. Paste a link, pick the exact quality you want, and watch it download — everything runs on your own computer.

[![CI](https://github.com/Sanoy24/ytgrab/actions/workflows/ci.yml/badge.svg)](https://github.com/Sanoy24/ytgrab/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/Sanoy24/ytgrab)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![YTGrab showing a download queue with live progress and download history](docs/images/screenshot.png)

## Features

- **Pick the real quality.** Paste a link and choose from the video's actual resolutions and audio streams, with sizes — or use a quick preset (best, 1080p, 720p, M4A, MP3).
- **Playlists, with a review step.** Browse any length of playlist 50 at a time, untick what you don't want, and confirm before anything is queued (up to 200 at once).
- **Live progress.** Speed, size, and time left for every download, updated as it happens.
- **Resume instead of restart.** Cancel, retry, or close the app mid-download — a retry continues from the partial file.
- **Sensible files.** Names include the quality (`Title [id] 1080p.mp4`); H.264 picks are saved as MP4.
- **Easy setup.** `ytgrab setup` installs everything it needs, asking first. `ytgrab doctor` explains what's missing.
- **Private by design.** The server listens only on `127.0.0.1` and keeps history in a local SQLite file. No accounts, no telemetry.

YTGrab is a single Go program. Downloading is done by [yt-dlp](https://github.com/yt-dlp/yt-dlp) and merging by [FFmpeg](https://ffmpeg.org/).

## Getting started

### macOS and Linux

With [Homebrew](https://brew.sh), which also installs yt-dlp, FFmpeg, and Deno:

```sh
brew install sanoy24/tap/ytgrab
ytgrab --open
```

Or, without Homebrew, run this in Terminal:

```sh
curl -fsSL https://raw.githubusercontent.com/Sanoy24/ytgrab/main/install.sh | sh
```

The installer picks the right build for your system, verifies its checksum, installs it into `~/.local/share/ytgrab`, adds a `ytgrab` command to `~/.local/bin`, and offers to run `ytgrab setup` to install yt-dlp, FFmpeg, and Deno. Then start YTGrab:

```sh
ytgrab --open
```

If `ytgrab` isn't found, the installer printed the line to add `~/.local/bin` to your `PATH`. Run the same command again to upgrade.

- **macOS:** `setup` installs FFmpeg and Deno with [Homebrew](https://brew.sh). The installer also adds **YTGrab** to `~/Applications`, so you can open it from Launchpad or Spotlight; it runs from the menu bar.
- **Linux:** `setup` installs yt-dlp and prints the package commands for the rest, for example `sudo apt install ffmpeg zenity unzip` and `curl -fsSL https://deno.land/install.sh | sh`. `zenity` (or `kdialog` on KDE) provides the folder window. The installer also adds **YTGrab** to your applications menu; started from there it runs in the tray on desktops that have one (KDE, Ubuntu, GNOME with the AppIndicator extension).

<details>
<summary>Install manually from the release archive instead</summary>

Download `ytgrab-<version>-darwin-arm64.tar.gz` (Apple silicon), `-darwin-amd64` (Intel Mac), `-linux-amd64`, or `-linux-arm64` from [Releases](https://github.com/Sanoy24/ytgrab/releases), then:

```sh
mkdir -p ~/.local/share/ytgrab
tar -xzf ~/Downloads/ytgrab-*.tar.gz -C ~/.local/share/ytgrab
cd ~/.local/share/ytgrab
chmod +x ytgrab
xattr -c ytgrab 2>/dev/null   # macOS: browser downloads are quarantined; the app isn't signed
./ytgrab setup
./ytgrab --open
```

</details>

### Windows

With [Scoop](https://scoop.sh), which also installs FFmpeg and Deno and adds YTGrab to the Start menu:

```powershell
scoop bucket add sanoy24 https://github.com/Sanoy24/scoop-bucket
scoop install sanoy24/ytgrab
ytgrab --open
```

Or install from the zip:

1. Download `ytgrab-<version>-windows-amd64.zip` from [Releases](https://github.com/Sanoy24/ytgrab/releases).
2. Right-click it → **Extract All…**, into a folder you keep (for example `C:\Apps\YTGrab`).
3. Double-click **Start YTGrab.cmd**. If Windows shows "Windows protected your PC", click **More info → Run anyway** (the app isn't code-signed).
4. The first start checks what's needed and offers to install anything missing — FFmpeg and Deno through `winget` — then opens YTGrab in your browser.

Next time, just double-click **Start YTGrab.cmd** again.

### With Go (any system)

```sh
go install github.com/Sanoy24/ytgrab/cmd/ytgrab@latest
ytgrab setup
ytgrab --open
```

Requires Go 1.25 or newer, with Go's `bin` folder (usually `~/go/bin`) on your `PATH`.

Whichever way you install, choose a download folder in the page and paste a link.

## Usage

| Command                       | What it does                                                               |
| ----------------------------- | -------------------------------------------------------------------------- |
| `ytgrab --open`               | Start the app and open it in your browser                                  |
| `ytgrab doctor`               | Check tools, folders, and the port                                         |
| `ytgrab setup`                | Install what's missing, asking before each step (`--yes` to accept all)    |
| `ytgrab setup --update-ytdlp` | Get the latest yt-dlp — the usual fix when YouTube downloads start failing |
| `ytgrab --version`            | Print the version                                                          |

On Windows, YTGrab started from the Start menu or **Start YTGrab.cmd** runs without a window: click its icon in the notification area by the clock to open the page, or right-click it and choose **Quit YTGrab**, or tick **Start with Windows** to have it waiting there after you sign in. Started from a terminal — and always on macOS and Linux — keep the terminal open while you use the app and press Ctrl+C to stop it. From a downloaded archive, run the commands from its folder as `./ytgrab …` (Windows: `.\ytgrab.exe …`). See the [user guide](docs/USER_GUIDE.md) for settings, environment variables, and troubleshooting.

## How it works

The browser page talks to a small local server. For each download, the server starts yt-dlp with a fixed list of arguments, reads its progress, and streams it to the page. Jobs and settings live in SQLite, so the queue and history survive restarts. Media goes straight from yt-dlp to your download folder; it never passes through the server or the browser.

More detail: [architecture](docs/ARCHITECTURE.md) · [local API](docs/API.md)

## Development

```sh
go test ./...
go run ./cmd/ytgrab --open
```

See [DEVELOPMENT.md](docs/DEVELOPMENT.md) for previewing the UI with sample data and opt-in integration tests, and [CONTRIBUTING.md](CONTRIBUTING.md) for reporting problems and sending changes. What's new in each version is in the [changelog](CHANGELOG.md).

## Responsible use

Only download videos you have the right to save — for example your own uploads, content under a permissive license, or where the platform and copyright holder allow it. You are responsible for complying with YouTube's Terms of Service and the laws that apply to you. YTGrab does not bypass sign-in, DRM, or paywalls.

## Acknowledgements

YTGrab stands on [yt-dlp](https://github.com/yt-dlp/yt-dlp), which does the hard work of talking to YouTube, and [FFmpeg](https://ffmpeg.org/). The job store uses [modernc.org/sqlite](https://gitlab.com/cznic/sqlite).

## License

[MIT]
