# Architecture

## Goal and boundaries

Build a responsive local downloader with a small Go application, a browser interface, a durable job queue, and clear dependency diagnostics. The application should remain useful when a download fails or the process restarts.

`yt-dlp` owns site extraction, format selection, network downloading, and resumable partial files. `ffmpeg`/`ffprobe` handle media merging and optional post-processing. A supported JavaScript runtime and `yt-dlp-ejs` may be needed for full YouTube support; dependency checks must report their availability rather than assuming every installation has them. The Go application owns orchestration and must not proxy video bytes through its HTTP server.

The first release targets one local user. Remote access, accounts, cloud storage, and multi-user permissions are outside its scope.

## Component map

```text
Browser UI
   | HTTP requests + server-sent progress events
   v
Go HTTP server (127.0.0.1)
   |-- API handlers and input validation
   |-- in-memory scheduler (bounded worker count)
   |-- SQLite store (jobs, settings)
   |-- yt-dlp adapter (child process + structured output parser)
   `-- dependency diagnostics
              |
              +-- yt-dlp -> download files directly to disk
              +-- ffmpeg/ffprobe -> merge or process media
              `-- supported JavaScript runtime, when needed
```

### Planned package layout

```text
cmd/ytgrab/              Application entry point
internal/app/            Startup, shutdown, and dependency wiring
internal/domain/         Job, options, state, and progress types
internal/config/         Configuration loading and validation
internal/downloader/     Downloader interface
internal/downloader/ytdlp/ yt-dlp process adapter and output parser
internal/queue/          Scheduling, concurrency, retry, cancellation
internal/store/sqlite/   Job and settings persistence
internal/api/            HTTP routes and SSE
internal/process/        Cross-platform child-process control
web/static/              Static, client-rendered UI (HTML, CSS, ES modules; no build step)
migrations/              Versioned SQLite schema
```

The UI is a static page that renders from the JSON API, so the Go server only needs to embed and serve `web/static/`; server-side templates are not planned. Proposed JSON shapes for integration are in `web/README.md`.

Keep the initial implementation simple. Add a package only when its boundary is useful; this layout is a target, not a requirement to create empty directories.

## Download flow

1. The browser submits a URL and a preset to `POST /api/jobs`.
2. The server validates the URL and options, stores a queued job, and returns its ID immediately.
3. A worker claims the job and starts `yt-dlp` with argument arrays, never a shell command string.
4. The adapter reads machine-oriented progress output line by line and updates the job. The UI receives updates through Server-Sent Events (SSE).
5. `yt-dlp` writes to the configured output directory. It invokes `ffmpeg` when separate streams need merging.
6. The worker records the final path, outcome, and any actionable error. On restart, an interrupted active job becomes `failed` with an `interrupted` error and can be retried using its preserved partial files; it is never left permanently "running."

Do not run a separate metadata inspection before every quick download. Inspect only when the user requests a format list or a workflow genuinely needs it.

## Job model and persistence

States: `queued`, `inspecting`, `downloading`, `processing`, `completed`, `failed`, `cancelled`. Valid transitions should be enforced in one place. A retry uses the same job ID, increments its attempt count, clears transient progress and error fields, and returns it to `queued`. Completed jobs cannot be retried.

Store at least the job ID, original URL, optional extracted video ID, requested preset and options, state, attempt count, progress bytes/total/speed/ETA where available, output path, error code/message, and timestamps. Progress may lack total bytes or ETA, so those fields must be nullable. Avoid writing every progress line to SQLite: throttle durable progress updates and broadcast more frequent transient events to connected browsers.

Use SQLite with WAL mode, migrations, short transactions, and a single-writer-friendly update pattern. Never place secrets in logs or ordinary job records. Cookies and authenticated downloads are deferred beyond the first release.

## Files and process safety

- Keep download bytes out of Go memory. Let `yt-dlp` write directly to disk and retain its normal `.part` files for resume.
- Use `os/exec` with a context and explicit arguments. Cancellation must stop the download process and any child processes; verify Windows and Unix behavior separately.
- Confine generated file paths to the configured download directory. Let `yt-dlp` apply its filename template and avoid constructing filenames from raw page titles in Go.
- Keep temporary files on the same volume as the destination when possible. A completed file should appear in the history only after the child process exits successfully and the final path is confirmed.
- Validate input as an HTTP(S) URL and reject unsupported hosts for the first release. Never pass arbitrary user-provided flags to `yt-dlp`.
- Bind to loopback by default. If remote binding is added later, design authentication and cross-site request protection before enabling it.

## Performance defaults

- Start with two concurrent jobs and four concurrent fragments per job. Make both configurable with conservative bounds.
- Queue excess jobs rather than spawning unlimited processes.
- Stream stdout/stderr incrementally; bound retained log output and keep only useful diagnostic context.
- Prefer remuxing/stream copy when possible. Audio conversion or video transcoding should be explicit because it consumes substantial CPU.
- Reuse `yt-dlp` retry/resume behavior, then add application-level retries only for classified transient failures.
- Keep metadata inspection and playlist expansion rate limited. Do not assume higher concurrency improves throughput against YouTube limits.

These are initial defaults, not performance claims. Measure throughput, CPU, memory, disk activity, and failure rate with representative downloads before tuning.

## Local API (planned)

| Method | Route | Purpose |
| --- | --- | --- |
| `POST` | `/api/jobs` | Create a download job |
| `GET` | `/api/jobs` | List recent jobs |
| `GET` | `/api/jobs/{id}` | Read a job |
| `POST` | `/api/jobs/{id}/cancel` | Cancel a queued or active job |
| `POST` | `/api/jobs/{id}/retry` | Retry a failed or cancelled job |
| `GET` | `/api/jobs/{id}/events` | Stream progress with SSE |
| `GET` | `/api/inspect?url=...` | Inspect available formats on demand |
| `GET`, `PUT` | `/api/settings` | Read or update local settings |
| `GET` | `/api/system/health` | Dependency and application status |

Use bounded request bodies, stable JSON error codes, and context-aware shutdown. A playlist URL should require explicit user confirmation and a maximum item count before creating multiple jobs.

The foundation health route returns `200` with `status` (`ready` or `degraded`), `checked_at`, a `dependencies` array, and a `note` explaining the limit of executable checks. Each dependency includes `name`, `required`, `available`, and an actionable `message`; available tools also include `path` and `version`. `degraded` means a required executable is missing or could not be run, while the local UI remains accessible for diagnostics. The server accepts `YTGRAB_LISTEN_ADDR` (loopback IP and port only, default `127.0.0.1:8787`) and `YTGRAB_TOOLS_DIR` (optional preferred binary directory). Deno is checked first, then Node; when Node is selected, the future downloader adapter must pass `--js-runtimes node` to yt-dlp.

## Dependency and release strategy

During development, find external binaries in a configured tools directory or `PATH` and report versions and missing capabilities. For packaged releases, distribute or install compatible binaries beside the application with a version manifest and verified checksums. Keep dependency update behavior separate from application updates because YouTube extraction may need more frequent changes.

## First release sequence

1. Bootstrap Go application, configuration, dependency diagnostics, and local HTTP health route.
2. Add SQLite schema, job states, and startup recovery.
3. Implement a single-download `yt-dlp` adapter with structured progress, cancellation, and output confirmation.
4. Add bounded queue, retry classification, and API endpoints.
5. Build the local web interface and SSE progress view.
6. Add audio presets, format inspection, and controlled playlist handling.
7. Test Windows startup/shutdown, cancellation, resume, and packaging; document installation and operation.

Track the concrete work in [PROGRESS.md](PROGRESS.md). Update that file at the end of each implementation iteration.
