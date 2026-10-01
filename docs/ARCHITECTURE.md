# Architecture

YTGrab is a single Go program that serves a browser UI on `127.0.0.1` and orchestrates [yt-dlp](https://github.com/yt-dlp/yt-dlp) and [FFmpeg](https://ffmpeg.org/). It is built for one local user: no accounts, no remote access, no cloud storage.

## Responsibilities

| Component | Owns |
| --- | --- |
| Browser UI (`web/static`) | Link entry, format and playlist review, queue and history, settings. Plain HTML, CSS, and ES modules embedded in the binary; no build step. |
| Go server (`internal/api`) | Input validation, the JSON API, progress streams (SSE), local-only request checks. |
| Queue (`internal/queue`) | A bounded pool of workers (two by default), cancellation, and automatic retry of network failures. |
| Store (`internal/store/sqlite`) | Jobs and settings in SQLite (WAL mode, versioned migrations embedded in the binary). |
| yt-dlp adapter (`internal/downloader/ytdlp`) | Building safe argument lists, parsing machine-readable progress, format inspection, playlist listing. |
| yt-dlp / FFmpeg | Extraction, downloading, resumable `.part` files, merging and conversion. |

```text
Browser ──HTTP + SSE──▶ Go server (127.0.0.1:8787)
                          ├─ API handlers + validation
                          ├─ queue (bounded workers)
                          ├─ SQLite (jobs, settings)
                          └─ yt-dlp adapter ──▶ yt-dlp ──▶ files on disk
                                                  └─▶ ffmpeg (merge / convert)
```

Media bytes never pass through the Go process or the browser: yt-dlp writes directly to the output folder.

## Package layout

```text
cmd/ytgrab/                  entry point and CLI (run, doctor, setup)
internal/app/                startup, shutdown, wiring, doctor checks
internal/app/deps/           finding tools and reading their versions
internal/api/                HTTP routes, SSE, request protection
internal/config/             environment configuration
internal/domain/             jobs, states, presets, URL parsing
internal/downloader/ytdlp/   yt-dlp process adapter, inspection, playlists
internal/picker/             the operating system's folder window
internal/process/            cross-platform process-tree control
internal/queue/              scheduling, cancellation, retries
internal/settings/           output-folder setting
internal/setup/              "ytgrab setup": tool installation
internal/store/sqlite/       persistence and migrations
web/static/                  the browser UI
```

## Download flow

1. The page submits a link with either a preset (`video-best`, `video-1080`, `video-720`, `audio-m4a`, `audio-mp3`) or a format picked from an inspection.
2. The server validates the link (YouTube hosts only, one video ID), stores a `queued` job, and returns it.
3. A worker starts yt-dlp with an explicit argument list — never a shell string — and the URL as the final argument after `--`.
4. yt-dlp prints progress through a custom `--progress-template`; the adapter parses it and updates the job. The page receives updates over Server-Sent Events.
5. The job completes only after yt-dlp exits successfully and the reported output file is confirmed to exist inside the output folder.

Output files are named `Title [video-id] 1080p.ext` (audio: `… 130k.ext`), so different qualities of one video never collide. A picked video format is paired with audio from the same container family (MP4 with M4A, WebM with WebM) so merges stay lossless.

### Job states

`queued → downloading → processing → completed`, with `failed` and `cancelled` reachable from any active state. Transitions are enforced in one place (`internal/domain`). Retry keeps the job ID, increments `attempt`, clears progress and errors, and returns the job to `queued`; completed jobs cannot be retried.

Jobs are stored as a JSON payload with indexed ID, state, video ID, and creation time. A partial unique index prevents two active jobs for the same video, and updates compare the previous state atomically. On startup, jobs left active by a closed app become `failed` with the `interrupted` code; Retry resumes them from yt-dlp's partial file.

## Format inspection

When a valid link is entered, the page asks the server to inspect it (`yt-dlp --dump-json`) and replaces the presets with the real options: one row per resolution and frame rate, and each audio stream. Inspection never blocks a download — presets stay usable and remain the fallback. The server serializes inspections, spaces them at least two seconds apart, and caches up to 64 results for ten minutes. A job created from an inspected format sends only a format kind and ID; the server checks it against its cached inspection and builds the yt-dlp format selector itself.

## Playlists

A playlist link is listed with one `--flat-playlist` request (at most 50 entries, sharing the inspection throttle). The user reviews the list and confirms with a button that states the count. The server validates every video ID before creating any job, builds each URL itself, and skips videos that are already queued. YouTube Mixes are refused because they are generated endlessly.

## Settings and the folder window

The output folder is stored in SQLite. On first run no folder is set and the page asks for one before downloading. Because browsers cannot reveal real folder paths, the server opens the operating system's own folder window on the user's desktop: Explorer's folder dialog on Windows (via PowerShell), `choose folder` on macOS, and zenity or kdialog on Linux. The dialog scripts are constants; the starting folder is passed through an environment variable. Typing a path is always available as a fallback.

## Security

- The server binds to loopback only and rejects requests whose `Host` is not a loopback name, which blocks DNS-rebinding pages.
- Cross-site writes are rejected using `Origin` and `Sec-Fetch-Site`.
- Links are validated before use; user input never becomes a yt-dlp option. Format IDs are checked against a strict pattern and a recent inspection.
- Output paths reported by yt-dlp must resolve inside the output folder.
- Request bodies and retained process output are size-limited.

## When YouTube limits the network

YouTube sometimes answers with HTTP 429 or a "confirm you're not a bot" check. Retrying straight away tends to extend the block, so a shared gate (`internal/cooldown`) pauses the download queue and format and playlist checks together: 15 minutes for the first block, then 30 and 60 while blocks continue, reset by the next successful download. A blocked job returns to the queue and runs when the pause ends, up to three attempts. Format checks during a pause answer immediately without contacting YouTube, and the job list reports `paused_until` so the page can show a countdown; `POST /api/system/resume` ends a pause early. To look less like a burst, consecutive downloads start 3–8 seconds apart and yt-dlp waits half a second between requests (`--sleep-requests`). The pause is kept in memory only.

## One copy at a time

On startup YTGrab claims its port before opening the database. If the port is taken by a running YTGrab (recognized by the `X-YTGrab-Version` response header), the new launch opens the browser to it and exits; it never runs startup recovery or starts workers against the running copy's jobs. An exclusive lock on `ytgrab.lock` in the data folder (`LockFileEx` on Windows, `flock` elsewhere) also stops a second copy on a different port from sharing the database. The lock is released automatically if the process ends.

## Processes, cancellation, and shutdown

Child processes run with a context. Cancellation stops the whole process tree (`taskkill /T` on Windows, a process group on Unix), and yt-dlp keeps its `.part` file so a retry resumes. On Ctrl+C or SIGTERM the server stops accepting requests, closes open progress streams, and cancels running jobs; they are marked `interrupted` on the next start.

## Tools and releases

Tools are found in this order: `YTGRAB_TOOLS_DIR`, a `tools` folder beside the program, a `tools` folder in the working directory, WinGet's `Links` folder on Windows, then `PATH`. Version checks allow 15 seconds (the Windows yt-dlp build unpacks itself on every run) and are cached per file.

yt-dlp is the part most likely to need updating, because YouTube changes often. The server looks up the latest yt-dlp release at most once a day (a single redirect request, in the background) and marks the installed one as outdated when a newer release exists. The tools panel and `ytgrab doctor` then offer an update, which installs the new version into YTGrab's own tools folder — never over a system-wide yt-dlp — with the same checksum verification as setup, and is refused while downloads are running.

`ytgrab doctor` reports tools, folders, and the port. `ytgrab setup` installs what is missing after asking: yt-dlp from its official GitHub release, verified against the published SHA-256 checksums; FFmpeg and Deno through winget or Homebrew; package-manager commands are printed on Linux.

`scripts/package.ps1` builds release archives. The Windows zip bundles a checksum-verified `yt-dlp.exe`; every archive includes `manifest.json` (versions and hashes) and `SHA256SUMS`. FFmpeg and a JavaScript runtime are installed separately because of their size and licensing.

## Defaults

| Setting | Value |
| --- | --- |
| Concurrent downloads | 2 |
| Fragments per download | 4 |
| Automatic retries | network failures, up to 3 attempts |
| YouTube block pauses | 15, 30, then 60 minutes; up to 3 attempts per job |
| Spacing between download starts | 3–8 seconds |
| Inspection cache | 64 entries, 10 minutes |
| Playlist limit | 50 videos |
| Shutdown timeout | 5 seconds |
