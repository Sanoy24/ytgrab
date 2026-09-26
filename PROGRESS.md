# Progress

This file is the source of truth for project progress. Update it after every iteration, including documentation or investigation work. A completed checklist item uses both `[x]` and strikethrough: `- [x] ~~Completed step.~~`.

## Milestones

### 0. Architecture and working agreements

- [x] ~~Choose Go application plus `yt-dlp` and `ffmpeg` integration.~~
- [x] ~~Document components, data flow, runtime boundaries, and performance defaults.~~
- [x] ~~Add agent instructions requiring an update after every iteration and strikethrough for completed steps.~~

### 1. Application foundation

- [ ] Initialize the Go module, entry point, configuration, and graceful shutdown.
- [ ] Add dependency discovery and a local health route with actionable diagnostics.
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

### 2026-09-26 — Architecture documentation

- Result: Established the initial Go plus `yt-dlp` architecture and created the repository guidance and progress checklist.
- Verification: Confirmed the repository was empty before adding the documents; documentation links and checklist conventions reviewed.
- Decision: Keep the first release local and single-user, with conservative concurrency and external tool diagnostics.
- Next: Bootstrap the Go application and dependency diagnostics in milestone 1.
