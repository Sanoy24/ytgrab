# Local YouTube Downloader

A local application for downloading videos and audio you are authorized to save. Go provides the browser interface, durable queue, and process control; `yt-dlp` handles downloads and `ffmpeg` handles merging and conversion.

Features: single videos with five quick presets or an exact resolution/audio stream picked from the video's real formats; playlists of up to 50 videos with a review list and explicit confirmation; live progress; cancel, retry, and resume from partial files; recovery after the app closes mid-download; persistent history; and a configurable output folder.

**Using a release?** See the [user guide](docs/USER_GUIDE.md) for installation, usage, and troubleshooting.

## Run locally

Install Go 1.25 or newer, `yt-dlp`, `ffmpeg`, and `ffprobe`. A JavaScript runtime such as Node or Deno is also recommended for YouTube extraction. Put the executables on `PATH` or place them in a directory selected with `YTGRAB_TOOLS_DIR`. The health panel reports missing tools.

From the repository root:

```powershell
go run ./cmd/ytgrab
```

Open `http://127.0.0.1:8787/` and choose a download folder. `go run ./cmd/ytgrab doctor` checks the tools and folders; `go run ./cmd/ytgrab setup` installs what's missing (yt-dlp into `tools/`, FFmpeg and Deno through winget or Homebrew), asking first. The server binds only to loopback. By default, the SQLite database lives in the user configuration directory under `ytgrab`, and completed files go to `Downloads/ytgrab`. Change the output folder in the page to any existing writable absolute folder; this affects jobs that start afterward and persists across restarts. Set `YTGRAB_DATA_DIR` and `YTGRAB_DOWNLOAD_DIR` before starting the app to choose initial locations; a saved output-folder setting takes precedence over `YTGRAB_DOWNLOAD_DIR`. `YTGRAB_LISTEN_ADDR` can select another loopback IP and port.

Each job downloads one YouTube video; a playlist becomes one job per confirmed video. The URL is passed as a single argument to `yt-dlp`, normal `.part` files are kept for resume, and media bytes never pass through the app's server. If the app closes mid-download, the job is marked interrupted on the next start and Retry resumes it. Flags: `--open` opens the browser, `--version` prints the version.

## Build a release

Download `yt-dlp.exe` and `SHA2-256SUMS` from the [yt-dlp releases](https://github.com/yt-dlp/yt-dlp/releases) into `tools/`, then:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\package.ps1 -Version 1.0.0 -AllPlatforms
```

Archives, `.sha256` files, and per-archive `manifest.json`/`SHA256SUMS` are written to `dist/`. The script runs the tests first (skip with `-SkipTests`) and refuses a `yt-dlp.exe` that doesn't match the official checksum.

## Tests

To run automated checks:

```powershell
go test ./...
```

The optional real-download test requires a `tools/yt-dlp.exe` (or corresponding executable on your platform) and internet access: set `YTGRAB_INTEGRATION=1`, then run `go test ./internal/app -run TestIntegrationDownload -v`.

## Project documents

- [docs/USER_GUIDE.md](docs/USER_GUIDE.md) is the end-user guide shipped in release archives.
- [web/README.md](web/README.md) describes the browser UI, its API contract, and fixture previews.
- [ARCHITECTURE.md](ARCHITECTURE.md) describes the components, data flow, runtime behavior, and design decisions.
- [PROGRESS.md](PROGRESS.md) tracks milestones and records each implementation iteration.
- [AGENTS.md](AGENTS.md) and [CLAUDE.md](CLAUDE.md) contain instructions for coding agents working in this repository.
- [DELEGATION.md](DELEGATION.md) assigns the first Codex and Claude tasks and explains the parallel branch workflow.

See [PROGRESS.md](PROGRESS.md) for the current implementation status.
