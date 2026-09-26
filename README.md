# Local YouTube Downloader

A local application for downloading videos and audio you are authorized to save. Go provides the browser interface, durable queue, and process control; `yt-dlp` handles downloads and `ffmpeg` handles merging and conversion.

The app is runnable, but it is still under development. Single-video downloads, five presets, cancellation, retries, progress, persistent history, and output-folder settings are implemented. Format inspection, playlists, packaging, and cross-platform verification remain in progress.

## Run locally

Install Go 1.25 or newer, `yt-dlp`, `ffmpeg`, and `ffprobe`. A JavaScript runtime such as Node or Deno is also recommended for YouTube extraction. Put the executables on `PATH` or place them in a directory selected with `YTGRAB_TOOLS_DIR`. The health panel reports missing tools.

From the repository root:

```powershell
go run ./cmd/ytgrab
```

Open `http://127.0.0.1:8787/`. The server binds only to loopback. By default, the SQLite database lives in the user configuration directory under `ytgrab`, and completed files go to `Downloads/ytgrab`. Change the output folder in the page to any existing writable absolute folder; this affects jobs that start afterward and persists across restarts. Set `YTGRAB_DATA_DIR` and `YTGRAB_DOWNLOAD_DIR` before starting the app to choose initial locations; a saved output-folder setting takes precedence over `YTGRAB_DOWNLOAD_DIR`. `YTGRAB_LISTEN_ADDR` can select another loopback IP and port.

The app accepts one YouTube video per job, not playlists. It passes the URL as a single argument to `yt-dlp`, retains normal `.part` files for possible resume, and never routes media bytes through the browser server. Do not close the app while jobs are running unless you are willing to retry them after restart.

To run automated checks:

```powershell
go test ./...
```

The optional real-download test requires a `tools/yt-dlp.exe` (or corresponding executable on your platform) and internet access: set `YTGRAB_INTEGRATION=1`, then run `go test ./internal/app -run TestIntegrationDownload -v`.

## Project documents

- [ARCHITECTURE.md](ARCHITECTURE.md) describes the components, data flow, runtime behavior, and design decisions.
- [PROGRESS.md](PROGRESS.md) tracks milestones and records each implementation iteration.
- [AGENTS.md](AGENTS.md) and [CLAUDE.md](CLAUDE.md) contain instructions for coding agents working in this repository.
- [DELEGATION.md](DELEGATION.md) assigns the first Codex and Claude tasks and explains the parallel branch workflow.

See [PROGRESS.md](PROGRESS.md) for the current implementation status.
