# Progress

This file is the source of truth for project progress. Update it after every iteration, including documentation or investigation work. A completed checklist item uses both `[x]` and strikethrough: `- [x] ~~Completed step.~~`.

## Milestones

### 0. Architecture and working agreements

- [x] ~~Choose Go application plus `yt-dlp` and `ffmpeg` integration.~~
- [x] ~~Document components, data flow, runtime boundaries, and performance defaults.~~
- [x] ~~Add agent instructions requiring an update after every iteration and strikethrough for completed steps.~~
- [x] ~~Define separate Codex and Claude worktrees, first task ownership, and integration procedure.~~

### 1. Application foundation

- [ ] Initialize the Go module, entry point, configuration, and graceful shutdown.
- [ ] Add dependency discovery and a local health route with actionable diagnostics.
- [x] ~~Build and preview a static local UI shell in `web/` (fixture data).~~
- [ ] Add an embedded minimal web page served on `127.0.0.1`.

### 2. Durable jobs

- [ ] Add versioned SQLite migrations and a job repository.
- [ ] Define and verify valid job state transitions and startup recovery.
- [ ] Add job creation, listing, and detail endpoints with input validation.

### 3. Download engine

- [ ] Integrate `yt-dlp` for a single video with safe arguments and confirmed output paths.
- [ ] Parse structured progress and expose it through SSE.
- [ ] Implement process-tree cancellation and verify it on supported platforms.
- [ ] Add a bounded worker queue, transient retry handling, and resume behavior.

### 4. Usable local interface

- [x] ~~Build URL submission form with client-side validation and video/audio preset choice (UI against fixtures).~~
- [ ] Connect URL submission and presets to `POST /api/jobs` and verify against the Go server.
- [ ] Add output-folder settings.
- [x] ~~Build queue/history views with progress, cancel, retry, and error/empty/loading states (UI against fixtures).~~
- [ ] Connect the queue/history views to the live API and SSE progress, and verify end to end.
- [ ] Add on-demand format inspection and controlled playlist downloads.

### 5. Release readiness

- [ ] Test end-to-end downloading, shutdown/recovery, cancellation, and resume.
- [ ] Document installation, required external tools, usage, and troubleshooting.
- [ ] Package and verify a local release on the supported operating systems.

## Iteration log

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
