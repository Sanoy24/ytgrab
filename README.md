# Local YouTube Downloader

A planned local application for downloading videos and audio you are authorized to save. The application will use Go for its HTTP interface, queue, persistence, and process control. It will use `yt-dlp` for extraction and downloading and `ffmpeg` for media merging and conversion.

The project is currently in the architecture stage. There is no runnable application yet.

## Project documents

- [ARCHITECTURE.md](ARCHITECTURE.md) describes the components, data flow, runtime behavior, and design decisions.
- [PROGRESS.md](PROGRESS.md) tracks milestones and records each implementation iteration.
- [AGENTS.md](AGENTS.md) and [CLAUDE.md](CLAUDE.md) contain instructions for coding agents working in this repository.
- [DELEGATION.md](DELEGATION.md) assigns the first Codex and Claude tasks and explains the parallel branch workflow.

## Planned first release

The first release will provide a local web interface for submitting a single YouTube URL, choosing a video or audio preset, watching progress, cancelling or retrying a job, and finding completed downloads. Jobs and settings will survive application restarts.

The application will listen on `127.0.0.1` by default. External tools will be discovered from a configured tools directory or `PATH`; dependency diagnostics will explain anything missing.

See [PROGRESS.md](PROGRESS.md) for the current implementation status.
