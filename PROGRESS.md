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
- [ ] Build and preview a static local UI shell in `web/`.
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

- [ ] Add URL submission, video/audio presets, and output-folder settings.
- [ ] Show live progress, history, cancellation, retries, and clear errors.
- [ ] Add on-demand format inspection and controlled playlist downloads.

### 5. Release readiness

- [ ] Test end-to-end downloading, shutdown/recovery, cancellation, and resume.
- [ ] Document installation, required external tools, usage, and troubleshooting.
- [ ] Package and verify a local release on the supported operating systems.

## Iteration log

### 2026-09-26 — Codex foundation

- Result: Added the Go module, loopback-only configuration, signal-aware HTTP startup/shutdown, and `/api/system/health` with executable version checks and install guidance. Documented the health response contract.
- Verification: `go test ./...` passed. A live request to `http://127.0.0.1:8787/api/system/health` returned `degraded` with missing `yt-dlp` guidance and detected FFmpeg, FFprobe, and Node. An integration test requests the health endpoint and verifies context-driven shutdown.
- Decision: Health remains HTTP 200 while dependencies are missing so the local UI can show diagnostics. Node requires `--js-runtimes node` when the downloader adapter is implemented.
- Limitation: The PTY interrupt did not terminate the `go run` processes cleanly, so the manual run did not verify OS-signal shutdown. The specific test processes were stopped; context-driven shutdown is covered by the integration test.
- Next: Hand this branch to the integrator. The static UI shell and its serving integration remain separate tasks.

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
