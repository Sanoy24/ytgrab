# Progress

This file is the source of truth for project progress. Update it after every iteration, including documentation or investigation work. A completed checklist item uses both `[x]` and strikethrough: `- [x] ~~Completed step.~~`.

## Milestones

### 0. Architecture and working agreements

- [x] ~~Choose Go application plus `yt-dlp` and `ffmpeg` integration.~~
- [x] ~~Document components, data flow, runtime boundaries, and performance defaults.~~
- [x] ~~Add agent instructions requiring an update after every iteration and strikethrough for completed steps.~~
- [x] ~~Define separate Codex and Claude worktrees, first task ownership, and integration procedure.~~

### 1. Application foundation

- [x] ~~Initialize the Go module, entry point, configuration, and graceful shutdown.~~
- [x] ~~Add dependency discovery and a local health route with actionable diagnostics.~~
- [x] ~~Build and preview a static local UI shell in `web/`.~~
- [x] ~~Add an embedded minimal web page served on `127.0.0.1`.~~

### 2. Durable jobs

- [x] ~~Add versioned SQLite migrations and a job repository.~~
- [x] ~~Define and verify valid job state transitions and startup recovery.~~
- [x] ~~Add job creation, listing, and detail endpoints with input validation.~~

### 3. Download engine

- [x] ~~Integrate `yt-dlp` for a single video with safe arguments and confirmed output paths.~~
- [x] ~~Parse structured progress and expose it through SSE.~~
- [x] ~~Implement process-tree cancellation and verify it on Windows.~~
- [ ] Verify process-tree cancellation on Unix.
- [x] ~~Add a bounded worker queue and transient network-error retry handling.~~
- [ ] Verify resume from yt-dlp partial files after interruption and retry.

### 4. Usable local interface

- [x] ~~Build URL submission form with client-side validation and video/audio preset choice (UI against fixtures).~~
- [ ] Connect URL submission and presets to `POST /api/jobs` and verify against the Go server.
- [ ] Add output-folder settings.
- [x] ~~Build queue/history views with progress, cancel, retry, and error/empty/loading states (UI against fixtures).~~
- [ ] Connect the queue/history views to the live API and SSE progress, and verify end to end.
- [ ] Add on-demand format inspection and controlled playlist downloads.

### 5. Release readiness

- [ ] Test end-to-end downloading, shutdown/recovery, cancellation, and resume.
- [x] ~~Document source-run setup, required external tools, basic usage, and dependency diagnostics.~~
- [ ] Document packaged installation and broader troubleshooting after release verification.
- [ ] Package and verify a local release on the supported operating systems.

## Iteration log

### 2026-09-26 — Post-merge checklist repair

- Result: Removed leftover merge-conflict text from the embedded-page checklist while preserving its completed status.
- Verification: The merged tree is clean on `master` before this edit; the embedded page has existing HTTP smoke and automated route checks recorded below.
- Next: Implement output-folder settings with validation and live API/UI integration.

### 2026-09-26 — Source-run handoff and HTTP smoke check

- Result: Replaced the architecture-stage README with source-run instructions, environment overrides, tool requirements, and current limitations. Tidied module requirements.
- Verification: A local Windows app process returned `ready` from `/api/system/health`; GET `/`, `/app.js`, and `/api/jobs` returned HTTP 200. `git diff --check`, JavaScript syntax checks, and `go vet ./...` passed. The test server process was stopped afterward.
- Limitation: Browser automation had no available browser session, so visual/live UI interaction remains unchecked. Packaged installation and wider troubleshooting remain future work.
- Next: Run a browser interaction pass, verify actual partial-file resume and Unix process cancellation, and implement settings/inspection/playlist scope.

### 2026-09-26 — Bounded transient retries

- Result: The two-worker queue now schedules the same job for a delayed retry after classified network errors, up to three total attempts. It persists the failed state first, leaving a manual retry path if the app exits before the timer fires.
- Verification: `go test ./... -count=1` passes, including a fake downloader that fails once with a network error, then completes on attempt two. Windows process-tree cancellation also remains green in the full suite.
- Decision: Do not automatically retry unavailable/private videos or generic failures. Resume from actual `.part` files remains to be verified separately.
- Next: Test recovery/resume with partial output, finish live UI and settings, then run a supported-platform release pass.

### 2026-09-26 — Windows process-tree cancellation

- Result: Added a process test with a parent spawning a heartbeat child, then cancelling the parent context. The child stopped writing, exercising the Windows `taskkill /T` path.
- Verification: `go test ./internal/process -run TestCancellationStopsProcessTree -v -count=1 -timeout 20s` passed on Windows.
- Limitation: The Unix process-group implementation is present but has not been executed on Unix. A live browser was unavailable, so UI visual interaction remains unchecked; a local HTTP health request returned `ready` with all required executables detected.
- Next: Add transient retry behavior and run broader recovery checks; later run the process test on Unix and the UI in a browser.

### 2026-09-26 — SSE progress delivery

- Result: Added a streaming HTTP test that receives an initial queued job and a later downloading event with persisted byte progress. The adapter's real-download test confirms the structured parser produces that progress.
- Verification: `go test ./...` passes, including the new SSE test.
- Decision: SSE emits persisted updates at the queue's throttled rate; the UI still needs a live browser check.
- Next: Drive the embedded page against the Go API, then test process-tree cancellation and recovery.

### 2026-09-26 — Real download and progress stream correction

- Result: Downloaded a public sample through the Go API, queue, and yt-dlp adapter, confirmed the output file, and fixed structured progress parsing to accept tagged events from either child-process stream.
- Verification: `go test ./...` passed. The opt-in `YTGRAB_INTEGRATION=1` app test downloaded a 309,156-byte M4A file and confirmed its title, output path, and persisted byte progress.
- Decision: Keep the SSE and live UI checklist items open until a client observes progress events end to end. Process-tree cancellation and resume also remain unverified.
- Next: Verify SSE delivery and live UI behavior, then test cancellation and retry/recovery paths.

### 2026-09-26 — Bounded workers and cancellation wiring

- Result: Added a two-worker scheduler that atomically claims queued jobs, persists throttled progress, marks download results, and propagates API cancellation to active work. Process helpers target the yt-dlp child tree on Windows and Unix.
- Verification: `go test ./...` passes, including a queue test that starts a fake downloader, durably cancels the active job, and observes context cancellation.
- Limitation: The real yt-dlp executable and FFmpeg child-process behavior have not yet been exercised. Automatic application-level retry and resume are not verified, so the combined queue checklist step remains open.
- Next: Add SSE progress delivery, then run the executable adapter and UI end to end.

### 2026-09-26 — yt-dlp adapter groundwork

- Result: Added safe yt-dlp argument construction for all five presets, machine-readable title/progress/final-path parsing, output confinement checks, bounded diagnostic errors, and platform-specific child-process termination helpers.
- Verification: `go test ./...` passes, including parser, URL-as-one-argument, preset-flag, and output-path checks. No actual download was run because `yt-dlp` is not installed locally yet.
- Decision: Keep the download checklist step open until a real or controlled executable run confirms the output path and process behavior.
- Next: Connect a two-worker queue and cancellation to persisted jobs, then exercise the adapter with an executable.

### 2026-09-26 — Job API and application persistence

- Result: The app opens SQLite at startup, recovers interrupted jobs, and exposes create/list/detail/cancel/retry routes. Requests validate YouTube video IDs and presets; errors use stable JSON codes. Mutating cross-origin requests are rejected.
- Verification: `go test ./...` passes, including a job API contract test for create, duplicate rejection, invalid input, list, detail, cancel, retry, and cross-origin rejection.
- Decision: `YTGRAB_DATA_DIR` and `YTGRAB_DOWNLOAD_DIR` can override the default user-local paths. Downloads remain queued until the worker is implemented.
- Next: Build the `yt-dlp` process adapter and bounded scheduler, then exercise the UI against running jobs.

### 2026-09-26 — Durable job model and storage

- Result: Added validated single-video URLs and presets, job state transitions, a pure-Go SQLite store with embedded versioned migration, duplicate-active-video prevention, and startup recovery of interrupted jobs.
- Verification: `go test ./...` passes, including persistence across database reopening, duplicate detection, recovery, retry state reset, URL validation, and transition checks.
- Decision: Store indexed job fields alongside a JSON snapshot so the API can return a stable job shape without extensive row mapping. SQLite uses WAL mode and one open connection to keep writes serialized.
- Next: Open the store during app startup and implement job creation, listing, detail, cancel, and retry routes.

### 2026-09-26 — Merge integration: embedded UI

- Result: Embedded `web/static` in the Go server, switched the UI default to the live HTTP client, retained `?fixture=...` previews, and aligned the health display with the Go response.
- Verification: `go test ./...` passes, including GET checks for `/`, `/app.js`, and `/app.css`; `node --check` passes for the edited JavaScript modules. Claude's branch log records the static UI preview and responsive checks.
- Decision: The page is served before job endpoints are available, so the UI currently shows the live API loading error until the next milestone is implemented.
- Next: Add durable job storage and the job API contract the UI already expects.

### 2026-09-26 — Claude: UI matches the health contract

- Result: On branch `agent/claude-ui-health`, the UI now reads Codex's `/api/system/health` shape (`available`, status `ready`/`degraded`). The header pill toggles a tools panel that lists every dependency with a short version, advice messages, and the report note; it opens automatically when a required tool is missing. Health fixtures were replaced with shapes captured from the Go server. In live mode, a 404/405 from the job routes now shows "Downloads aren't available yet" and stops polling instead of repeating requests. Adopted the live-by-default client switch (`?fixture=...` for sample data), matching Codex's uncommitted change on `master`. Re-checked the milestone 1 UI-shell item that was unchecked during the merge.
- Verification: Built `cmd/ytgrab` into a temp folder and ran it on 127.0.0.1:8787; `curl` of `/api/system/health` returned `degraded` (yt-dlp missing; ffmpeg/ffprobe 8.0.1; Node 24.4.1). Served `web/static` with a scratch Node server that proxies `/api/*` to Go, then checked in Playwright: the live page showed "1 tool missing" and the four real tools; `/api/jobs` was requested twice in 6 s (initial load plus one poll), then polling stopped; a submit showed the "not available" message. Fixture modes: default shows "Tools ready" with the panel collapsed and opens on click; degraded shows "2 tools missing" at 375px with no horizontal overflow (`scrollWidth` 360); error shows "Server offline". `node --check` passes. No Go files were edited.
- Blocker/notes: Codex has uncommitted changes on `master` (`web/assets.go`, the health handler, and a smaller version of this fix in `web/static/app.js`/`api.js`/`web/README.md`). `api.js` and the README now include Codex's edits verbatim; `loadHealth` in `app.js` will conflict, and this branch's version supersedes it.
- Next: After Codex commits the embedding work, rebase this branch onto `master`, resolve `app.js` by keeping this version, and verify the page served by Go at `http://127.0.0.1:8787/`.

### 2026-09-26 — Codex foundation

- Result: Added the Go module, loopback-only configuration, signal-aware HTTP startup/shutdown, and `/api/system/health` with executable version checks and install guidance. Documented the health response contract.
- Verification: `go test ./...` passed. A live request to `http://127.0.0.1:8787/api/system/health` returned `degraded` with missing `yt-dlp` guidance and detected FFmpeg, FFprobe, and Node. An integration test requests the health endpoint and verifies context-driven shutdown.
- Decision: Health remains HTTP 200 while dependencies are missing so the local UI can show diagnostics. Node requires `--js-runtimes node` when the downloader adapter is implemented.
- Limitation: The PTY interrupt did not terminate the `go run` processes cleanly, so the manual run did not verify OS-signal shutdown. The specific test processes were stopped; context-driven shutdown is covered by the integration test.
- Next: Hand this branch to the integrator. The static UI shell and its serving integration remain separate tasks.

### 2026-09-26 — Claude: static UI shell

- Result: Added the static, client-rendered UI in `web/static/` (branch `agent/claude-ui-shell`): URL input with paste button and client-side YouTube link checks; five presets (video best/1080p/720p, audio M4A/MP3); queue with determinate and indeterminate progress, speed, ETA, and cancel; history with filters, output path copy, retry, and error hints; dependency health pill and missing-tool banner; empty, loading, load-error, and offline states; light/dark themes. Data comes from `api.js`, which uses fixtures by default (`?fixture=default|empty|error|degraded|loading`) and has an untested HTTP client (`?api=live`) for the planned routes. `web/README.md` documents preview steps and proposes JSON shapes for jobs, errors, and health.
- Verification: Served `web/static` with `python -m http.server` on 127.0.0.1 and drove it with Playwright (Chromium). Checked desktop 1280px and 1024px, phone 375px, and dark mode by screenshot; confirmed no horizontal overflow at 375px (`scrollWidth` 360) after fixing a stacked-row overflow. Scripted checks: six invalid URL cases show the expected messages; a scheme-less video link with `list=` is accepted with the playlist note; cancel moves a job to history; retry re-queues and increments the attempt; history filters show the right states; keyboard focus survives live progress updates; the error scenario shows load errors, the offline banner, and a failing-submit toast that keeps the URL; loading shows skeletons with `aria-busy`. No console errors after adding an inline favicon. `node --check` passes on all modules.
- Decision: Use a static client-rendered page with no build step instead of server templates; recorded in `ARCHITECTURE.md`. The JSON shapes in `web/README.md` are a proposal for Codex/the integrator, not a final contract. No Go files were touched.
- Blocker/notes: The main checkout was on `agent/codex-foundation` with uncommitted coordination docs, so this branch was created from `master` in `.worktrees/claude` and this `PROGRESS.md` started from the uncommitted working copy. The HTTP client, SSE, and server-side validation are not verified.
- Next: After the Go server lands, embed and serve `web/static/`, align the JSON shapes, switch the default client to live, and verify submission, progress, cancel, and retry against real jobs.

### 2026-09-26 — Delegation setup

- Result: Added the first parallel assignments, branch/worktree procedure, agent prompts, and merge rules in `DELEGATION.md`.
- Verification: Confirmed the repository has a first commit and both `codex` and `claude` commands are installed. Reviewed path ownership so the first coding tasks do not overlap.
- Decision: Both agents update `PROGRESS.md` in their own branches; the integrator combines the logs during merge.
- Next: Commit the coordination docs, create the two worktrees, and launch each agent with its scoped prompt.

### 2026-09-26 — Architecture documentation

- Result: Established the initial Go plus `yt-dlp` architecture and created the repository guidance and progress checklist.
- Verification: Confirmed the repository was empty before adding the documents; documentation links and checklist conventions reviewed.
- Decision: Keep the first release local and single-user, with conservative concurrency and external tool diagnostics.
- Next: Bootstrap the Go application and dependency diagnostics in milestone 1.
