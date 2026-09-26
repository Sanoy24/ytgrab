# YTGrab

A local YouTube downloader with a clean browser interface. Paste a link, pick the exact quality you want, and watch it download — everything runs on your own computer.

[![CI](https://github.com/Sanoy24/ytgrab/actions/workflows/ci.yml/badge.svg)](https://github.com/Sanoy24/ytgrab/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/Sanoy24/ytgrab)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![YTGrab showing a download queue with live progress and download history](docs/images/screenshot.png)

## Features

- **Pick the real quality.** Paste a link and choose from the video's actual resolutions and audio streams, with sizes — or use a quick preset (best, 1080p, 720p, M4A, MP3).
- **Playlists, with a review step.** See up to 50 videos, untick what you don't want, and confirm before anything is queued.
- **Live progress.** Speed, size, and time left for every download, updated as it happens.
- **Resume instead of restart.** Cancel, retry, or close the app mid-download — a retry continues from the partial file.
- **Sensible files.** Names include the quality (`Title [id] 1080p.mp4`); H.264 picks are saved as MP4.
- **Easy setup.** `ytgrab setup` installs everything it needs, asking first. `ytgrab doctor` explains what's missing.
- **Private by design.** The server listens only on `127.0.0.1` and keeps history in a local SQLite file. No accounts, no telemetry.

YTGrab is a single Go program. Downloading is done by [yt-dlp](https://github.com/yt-dlp/yt-dlp) and merging by [FFmpeg](https://ffmpeg.org/).

## Getting started

### Windows

1. Download the latest `ytgrab-<version>-windows-amd64.zip` from [Releases](https://github.com/Sanoy24/ytgrab/releases) and unzip it into a folder you keep.
2. Double-click **Start YTGrab.cmd**.

The first start checks what's needed and offers to install anything missing (FFmpeg and Deno through `winget`), then opens YTGrab in your browser. Choose a download folder and paste a link.

### macOS and Linux

Download the archive for your platform from [Releases](https://github.com/Sanoy24/ytgrab/releases), or install with Go 1.25+:

```sh
go install github.com/Sanoy24/ytgrab/cmd/ytgrab@latest
```

Then:

```sh
ytgrab setup    # installs yt-dlp; FFmpeg and Deno via Homebrew, or prints the commands on Linux
ytgrab --open   # starts YTGrab and opens http://127.0.0.1:8787/
```

## Usage

| Command | What it does |
| --- | --- |
| `ytgrab --open` | Start the app and open it in your browser |
| `ytgrab doctor` | Check tools, folders, and the port |
| `ytgrab setup` | Install what's missing, asking before each step (`--yes` to accept all) |
| `ytgrab setup --update-ytdlp` | Get the latest yt-dlp — the usual fix when YouTube downloads start failing |
| `ytgrab --version` | Print the version |

Keep the console window open while you use the app; press Ctrl+C to stop it. See the [user guide](docs/USER_GUIDE.md) for settings, environment variables, and troubleshooting.

## How it works

The browser page talks to a small local server. For each download, the server starts yt-dlp with a fixed list of arguments, reads its progress, and streams it to the page. Jobs and settings live in SQLite, so the queue and history survive restarts. Media goes straight from yt-dlp to your download folder; it never passes through the server or the browser.

More detail: [architecture](docs/ARCHITECTURE.md) · [local API](docs/API.md)

## Development

```sh
go test ./...
go run ./cmd/ytgrab --open
```

See [DEVELOPMENT.md](docs/DEVELOPMENT.md) for previewing the UI with sample data, opt-in integration tests, and building release archives. Issues and pull requests are welcome.

## Responsible use

Only download videos you have the right to save — for example your own uploads, content under a permissive license, or where the platform and copyright holder allow it. You are responsible for complying with YouTube's Terms of Service and the laws that apply to you. YTGrab does not bypass sign-in, DRM, or paywalls.

## Acknowledgements

YTGrab stands on [yt-dlp](https://github.com/yt-dlp/yt-dlp), which does the hard work of talking to YouTube, and [FFmpeg](https://ffmpeg.org/). The job store uses [modernc.org/sqlite](https://gitlab.com/cznic/sqlite).

## License

[MIT](LICENSE) © 2026 Yonas Mekonnen
