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

- [x] ~~Classify YouTube bot checks and rate limiting with an actionable `blocked` error.~~
- [x] ~~Integrate `yt-dlp` for a single video with safe arguments and confirmed output paths.~~
- [x] ~~Parse structured progress and expose it through SSE.~~
- [x] ~~Implement process-tree cancellation and verify it on Windows.~~
- [x] ~~Verify process-tree cancellation on Unix.~~
- [x] ~~Add a bounded worker queue and transient network-error retry handling.~~
- [x] ~~Verify resume from yt-dlp partial files after interruption and retry.~~
- [x] ~~Name output files by quality so a second pick of the same video is not reported as already downloaded.~~
- [x] ~~Pair picked video with audio of the same container (H.264 + AAC stays MP4).~~

### 4. Usable local interface

- [x] ~~Return sentence-case output-folder validation errors.~~
- [x] ~~Build URL submission form with client-side validation and video/audio preset choice (UI against fixtures).~~
- [x] ~~Connect URL submission and presets to `POST /api/jobs` and verify against the Go server.~~
- [x] ~~Add persisted output-folder settings, API, and page controls.~~
- [x] ~~Verify output-folder controls in a live browser session.~~
- [x] ~~Add the output-folder setting to the UI against `GET`/`PUT /api/settings`.~~
- [x] ~~Build queue/history views with progress, cancel, retry, and error/empty/loading states (UI against fixtures).~~
- [x] ~~Connect the queue/history views to the live API and SSE progress, and verify end to end.~~
- [x] ~~Label the video and audio parts of a merged download's progress.~~
- [x] ~~Require a download folder on first run and choose it with the operating system's folder window.~~
- [x] ~~Add `ytgrab doctor` and `ytgrab setup` for one-step tool installation.~~
- [x] ~~Build the format picker UI: inspect on paste, grouped video/audio choices, preset fallback (against fixtures).~~
- [x] ~~Add `GET /api/inspect` and server-validated format-based job creation.~~
- [x] ~~Verify the format picker end to end in a live browser.~~
- [x] ~~Add controlled playlist downloads (review list, 50-item cap, explicit count confirmation, Mix refusal).~~

### 5. Release readiness

- [x] ~~Test end-to-end downloading, crash recovery, cancellation, and resume on Windows.~~
- [x] ~~Verify graceful shutdown (SIGINT) with an active download and an open progress stream (Linux; Windows Ctrl+C uses the same code path).~~
- [x] ~~Document source-run setup, required external tools, basic usage, and dependency diagnostics.~~
- [x] ~~Document packaged installation and broader troubleshooting after release verification.~~
- [x] ~~Add a packaging script with checksum-verified yt-dlp, manifest, and per-file checksums.~~
- [x] ~~Package and verify the Windows x64 release (fresh extract: checksums, manifest, start script, tool discovery, health, page).~~
- [x] ~~Package the Linux x64 release and verify it runs and serves the page (WSL2 Ubuntu).~~
- [ ] Complete a real YouTube download through the packaged Windows build (blocked by YouTube rate limiting during this session).
- [ ] Verify the macOS archives on a Mac (built, not run).

## Iteration log

### 2026-09-27 — Claude: folder window, first-run folder choice, doctor and setup

- Result: On branch `claude/setup-and-picker` (stacked on `claude/finish`). The page now asks for a download folder before the first download: **Choose folder…** opens the operating system's folder window through the server (`internal/picker`: Explorer's `IFileOpenDialog` via PowerShell on Windows, `choose folder` on macOS, zenity/kdialog on Linux), **Use Downloads\ytgrab** creates the suggested folder, and typing a path remains a fallback. Later, **Change…** opens the same window. Settings expose `configured`, `default_dir`, `can_pick`; new routes `POST /api/settings/pick-folder` (one window at a time; cancel is not an error) and `/use-default`. Added `RequireLoopbackHost` so a DNS-rebinding page can't reach the API or open windows. New CLI: `ytgrab doctor` (tools with short versions, data folder, download folder, port) and `ytgrab setup [--yes] [--update-ytdlp]` (official yt-dlp download with SHA-256 verification into `tools/`; FFmpeg and Deno through winget or Homebrew after asking; printed commands on Linux; re-checks afterwards). Tool search includes WinGet's `Links` folder. `Start YTGrab.cmd` runs `doctor` and offers `setup` when something is missing. Updated the user guide, README, and ARCHITECTURE.
- Verification: `go test ./...` and `go vet ./...` pass, with new tests for first-run settings, the pick/cancel/unavailable/use-default routes, the loopback Host check, picker commands (initial folder only via environment, zenity/kdialog choice, cancel), the checksum-verified yt-dlp download (mismatch leaves nothing behind), and setup prompts (Windows runs winget only for accepted items and prints the command for declined ones; Linux runs nothing). Real Windows folder window, driven by UI automation: opened in about 3–4 s and returned a folder; Esc returned "cancelled". From the Go-served page with a fresh data folder: the first-run card showed, **Choose folder…** read "Waiting for folder window…", the real window opened, and the chosen folder was saved (`configured: true`). `ytgrab doctor` on this machine: all tools found, download folder not chosen, port free. `ytgrab setup` in an empty folder downloaded yt-dlp 2026.08.19 from GitHub, verified its checksum, and the re-check passed. Fixture runs covered first-run blocking of Add, cancel, pick, Change…, and the no-picker fallback.
- Blocker/notes: The macOS and Linux folder windows are unit-tested but not run. `Start YTGrab.cmd` was not double-clicked here because it opens the desktop browser. My earlier `--version` test had created an empty `%AppData%\ytgrab\jobs.db` (no settings, no jobs); the app creates the same file on first start.
- Next: Merge `claude/finish`, then this branch; build a release and try one real download when YouTube allows.

### 2026-09-26 — Claude: release packaging, user guide, tool-check fix

- Result: Added `scripts/package.ps1` (tests, trimmed version-stamped builds, yt-dlp bundled only if it matches the official `SHA2-256SUMS`, `manifest.json`, per-file `SHA256SUMS`, archive `.sha256`; zips built with `tar -a` because Windows PowerShell's `Compress-Archive` writes backslash entry names), `packaging/Start YTGrab.cmd` (CRLF, enforced by `.gitattributes`), `--open` and `--version` flags, a startup line with version and "Press Ctrl+C to stop.", and `docs/USER_GUIDE.md` (install, usage, playlists, settings, troubleshooting), shipped as the archive README. **Fixed tools flapping to "missing":** the dependency version check timed out at 3 s, and the Windows `yt-dlp.exe` takes about 3.0 s per run; the limit is now 15 s and successful checks are cached per file path, size, and modification time. Updated `README.md` and the release strategy in `ARCHITECTURE.md`; `dist/` is ignored.
- Verification: `go test ./...` and `go vet ./...` pass, including new tests for the browser command, the version-check timeout, and the cache (the test binary acts as a fake tool; one run for two checks, a second run after the file's time changes). Built `1.0.0-rc1` for Windows, Linux, and macOS (arm64/amd64). Windows zip (rebuilt as `rc2` after the fix): `sha256sum -c` passes for the archive and every file; the manifest records yt-dlp 2026.08.19 with the official hash; `ytgrab.exe --version` prints the version; run from a fresh extract with no tools variable, it found `tools\yt-dlp.exe` beside itself, and the first health check (3.4 s) reported it available, then 0.06 s from cache (with `rc1` it was reported unavailable after a timeout); the page and scripts load. Linux archive in WSL2: checksums pass, `--version`, page 200, health reports missing tools, SIGINT exits cleanly. A real download through the Windows release returned `blocked` because YouTube was rate-limiting this network.
- Blocker/notes: `Start YTGrab.cmd`/`--open` were not launched here (they would open the desktop browser); the command is unit-tested. macOS archives are built but not run. The Unix archives don't keep the executable bit; the guide says to `chmod +x`.
- Next: When YouTube allows, run one download through the packaged Windows build; merge `claude/finish`.

### 2026-09-26 — Claude: graceful shutdown fix, Unix verification

- Result: **Fixed shutdown hanging on open progress streams:** `server.Shutdown` waited for SSE handlers, which only end when their request context does, so with a browser tab open Ctrl+C took the full 5 s timeout and exited 1 with `shutdown: context deadline exceeded`. Request contexts now derive from a base context cancelled via `RegisterOnShutdown`. Added `internal/app/shutdown_test.go`.
- Verification: The new test failed before the fix (`serve returned shutdown: context deadline exceeded` after 3 s) and passes after; `go test ./...` and `go vet ./...` pass. In WSL2 Ubuntu (Linux 6.18) with cross-compiled binaries and a fake `yt-dlp` shell script that prints progress and holds a `sleep` child: the fake ran in its own process group with its child, and Cancel left no fake or `sleep` process; the cross-compiled `internal/process` and `internal/queue` test suites pass on Linux; SIGINT with an active download and an open `curl -N` event stream exited 0 in 0.03 s (before the fix: exit 1 after 5.02 s), left no child processes, and after restart the job was `failed`/`interrupted`.
- Blocker/notes: Windows Ctrl+C was not sent directly (no simple way from this harness); it goes through the same `signal.NotifyContext` path verified on Linux.
- Next: Packaging script and a verified Windows release zip, then install and troubleshooting docs.

### 2026-09-26 — Claude: controlled playlist downloads

- Result: Added `domain.ParsePlaylistURL` (canonical `/playlist?list=` URL, Mixes refused), `MaxPlaylistItems = 50`, `VideoURL`, and `ValidVideoID`. `Inspector.ListPlaylist` runs one `--flat-playlist --dump-single-json --playlist-items 1:51` request through the shared throttle (extracted as `throttle`, with a shared bounded `runJSON`), skips private/deleted placeholders, and reports `truncated`/`unavailable`. New routes `GET /api/playlist` and `POST /api/playlist/jobs` validate all IDs before creating any job, build URLs server-side, and skip duplicates. Listed and inspected titles are remembered so new jobs show titles while queued. "The playlist does not exist" is now `video_unavailable` with a playlist message. The page shows a review list with select-all, notes for skipped/truncated entries, and an "Add N videos" button; video links with `list=` offer the whole playlist. Documented in `ARCHITECTURE.md` and `web/README.md`.
- Verification: `go test ./...` and `go vet ./...` pass, with new tests for URL parsing (including Mixes and injected characters), flat-listing parsing and the 50 cap, listing arguments, the API contract (nothing created on bad input, >50 rejected, preset required, duplicates skipped, canonical URLs, titles applied), title memory, and the missing-playlist classification. Fixture run in Chromium: Add before loading only opened the review; 12 entries with a "2 private or deleted" note; unticking updated the button to "Add 10 videos" with an indeterminate select-all; submit created jobs and reset the form; Mix refused; the whole-playlist offer switched modes; editing the link left playlist mode. Live against a branch build: `/api/playlist` listed Blender Studio's "Project Gold" (7 entries) in 4.4 s and "Blender Studio Logs" (31, not truncated); a Mix returned `mix_playlist`; in the Go-served page, choosing 2 of 7 as M4A created 2 jobs with the right IDs. Those downloads failed with `blocked` because YouTube was rate-limiting this network at the time.
- Blocker/notes: A successful live playlist download still needs a quieter network moment; the single-video download path it uses is already verified.
- Next: Graceful shutdown and Unix checks (WSL), packaging, and install/troubleshooting docs.

### 2026-09-26 — Claude: live end-to-end pass, quality file names, MP4 pairing

- Result: On branch `claude/finish` (now working on backend and frontend). Picked video is paired with same-container audio (`ID+ba[ext=m4a]/ID+ba/ID` for MP4, `[ext=webm]` for WebM) and merges with `mp4/webm/mkv`; the inspected container travels in `FormatSelection.ext`, and audio labels include bitrate. **Fixed a wrong-file bug found live:** every quality of a video shared one file name, so after downloading 480p, a 1080p job finished instantly and pointed at the 480p file; names now include `%(height)sp` for video and `%(abr).0fk` for M4A/audio picks. Progress events carry `stream` (video/audio) from `%(info.vcodec)j`, and the page labels the part and shows plain "Starting…" before bytes arrive. The live client refreshes every 2 s while work is active and 10 s when idle, refreshes immediately after add/cancel/retry, guards against overlapping polls, and caps progress streams at 4.
- Verification: `go test ./...` and `go vet ./...` pass; new tests cover the audio pairing, output names, and stream parsing; all web modules pass a `.mjs` syntax check. Live on Windows with a branch build (temp data dir, repo `tools/`): saved an output folder (bad path rejected with the sentence-case message) and it persisted across restart; Big Buck Bunny inspection took 7 s and listed 8 resolutions; a 480p H.264 pick streamed progress every 1–2 s; Cancel at 84% stopped both yt-dlp processes and kept a 23.9 MB `.part`; Retry resumed at 86%, fetched audio, merged, and ffprobe shows H.264 + AAC in `.mp4` (38.8 MB, 634 s); force-killing ytgrab during a 473 MB 1440p download also ended yt-dlp (the `.part` stopped at 10.1 MB); after restart the job was `interrupted` and Retry resumed at 14 MB. The file-name bug was observed before the fix (1080p job done, file was 480p per ffprobe); the fix is covered by unit tests but not yet re-run live because YouTube began rate-limiting this network.
- Blocker/notes: yt-dlp exits on a broken pipe when ytgrab dies, which covers downloads; during a silent ffmpeg merge it may outlive ytgrab. Graceful Ctrl+C shutdown with an active download is not yet verified.
- Next: Controlled playlist downloads, then graceful-shutdown and Unix checks (WSL), packaging, and install/troubleshooting docs.

### 2026-09-26 — Claude: fix broken page on master, live format downloads

- Result: On branch `agent/claude-inspect-expiry` (from `master` at `ea5b079`). **Fixed a page-breaking bug on `master`:** merging both agents' output-folder UIs left two top-level `loadSettings` functions in `web/static/app.js`, which browsers reject in ES modules (`Identifier 'loadSettings' has already been declared`), so no script ran. Removed the unwired duplicate form, functions, client methods, and CSS, keeping the tested `#output-dir` UI. The page's inspection cache now expires after 9 minutes (server: 10), and an `inspection_required` response triggers one re-check and retry with the same choice. YouTube `-drc` audio copies are hidden when the original exists (a real video went from 8 to 5 audio rows). Added the `expired` fixture and documented a module-aware syntax check in `web/README.md`.
- Verification: `go test ./...` passed on `master`. A `.mjs` copy of `master`'s `app.js` fails `node --check` with the duplicate-declaration error; all four branch modules pass. No duplicate element IDs or references to missing elements. Fixture `expired`: submit was rejected, the page re-checked, retried, and added "Video · 720p" with no console errors. Live, with a `master` build (temp data/download dirs) behind a scratch proxy serving this branch: health "Tools ready", output folder shown, pasting `watch?v=jNQXAC9IVRw` listed "Me at the zoo", 2 video rows (240p, 144p) and 5 audio rows plus MP3. Picking AAC 130 kbps downloaded `Me at the zoo [jNQXAC9IVRw].m4a` (309,156 bytes) in about 6 s; picking 144p went Downloading → Processing → Done and wrote a 446,848-byte `.mkv`. Temp files were removed afterwards.
- Blocker/notes: `master` serves a non-working page until this branch is merged. H.264 picks merge with Opus audio into `.mkv`; `web/README.md` suggests preferring M4A audio for `avc1` video to get `.mp4` (a Go change). Live output-folder saving was verified earlier against Codex's pre-commit API, not re-run on the committed code.
- Next: Merge this branch promptly. Then move queue progress from list polling to per-job SSE, and re-verify output-folder saving on `master`.

### 2026-09-26 — Inspection and exact-format backend

- Result: Added bounded, serialized and briefly cached yt-dlp inspection for one validated video URL. The API returns filtered video-only/audio-only formats and accepts a format job only when its safe ID and kind match a recent server-side inspection. The server constructs the final yt-dlp selector; jobs persist `preset: null` and a server-made format label.
- Verification: `go test ./... -count=1`, `go vet ./...`, and `git diff --check` pass. Tests cover parser, selector, rejected free-form/uninspected IDs, API shape, and blocked errors. The opt-in metadata-only integration test inspected a real public video and returned 12 video-only and 10 audio-only formats.
- Verification added: The opt-in selected-format integration test inspected the same public video, selected M4A format `139-drc`, and downloaded a confirmed 117,454-byte file.
- Limitation: Live browser picker interaction remains unverified. Controlled playlist downloads remain separate. Reconciled a duplicate unfinished output-folder checklist line left by merged progress logs.
- Next: Verify selected-format downloading through the API and run a browser picker pass when a browser is available.

### 2026-09-26 — README error-contract fixes

- Result: Classified yt-dlp HTTP 429 and bot-confirmation failures as `blocked`, with an actionable wait-and-retry message. Output-folder validation now returns a sentence-case error for direct UI display.
- Verification: Targeted settings and yt-dlp tests pass, including bot-check classification tests.
- Next: Implement bounded on-demand inspection and validate exact format IDs before queuing format-specific jobs.

### 2026-09-26 — Output-folder API and page controls

- Result: Added a page field for the output folder, fixture/live client methods, and HTTP contract tests; restored SSE subscriptions in the merged live client, which had regressed to polling-only. Updated source-run and web contract docs.
- Verification: `go test ./... -count=1` passes. A built local server returned the initial folder, accepted a `PUT /api/settings`, returned the saved folder on a subsequent GET and after a process restart, and served the page containing the form. `node --check` and `git diff --check` passed.
- Limitation: The browser-testing skill found no browser session, so visual and interactive UI behavior remains unchecked. The settings smoke server used isolated ignored data/output directories.
- Next: Run browser verification when available; test real partial-file resume and complete remaining inspection/playlist work.

### 2026-09-26 — Persisted output-folder backend

- Result: Added a versioned SQLite settings table, a synchronized output-folder manager, and `GET`/`PUT /api/settings`. New jobs read the current folder once at process start, so changing it cannot move a running download mid-stream. The setter requires an existing writable absolute directory.
- Verification: `go test ./...` passes, including persistence and invalid-folder tests. The UI and HTTP settings contract have not yet been tested, so the checklist remains open.
- Next: Add API contract coverage and the page controls, then run an HTTP and browser pass if a browser is available.

### 2026-09-26 — Claude: format picker on paste

- Result: On branch `agent/claude-format-picker` (stacked on `agent/claude-post-merge`), pasting or typing a valid video link now fetches its formats and replaces the quick presets with real choices: video rows grouped by resolution and frame rate (H.264/MP4 preferred, sizes estimated with best audio), each audio stream, and MP3 conversion. The previously chosen preset carries over to the closest format; results are cached per link; stale requests are aborted; failures keep the presets with the reason and Try again. Jobs created from a format show its label. Added `web/static/formats.js`, an inspection fixture modeled on a real YouTube format list, a `blocked` fixture scenario, and an API proposal in `web/README.md`. Updated `ARCHITECTURE.md`: inspection now runs on link entry (user request), never blocks a download, and should be cached and rate limited server-side.
- Verification: Ran `formats.js` in Node against the fixture: 14 video streams became 10 rows, 5 audio rows, and presets map to 2160p/1080p60/720p60/M4A 130 kbps (MP3 stays a conversion). In Playwright (Chromium) with fixtures: paste showed "Checking available formats…" then the title, duration, and lists; "Up to 1080p" carried over to 1080p 60fps; picking M4A 130 kbps created a job labeled "Audio · M4A 130 kbps" and reset the form; a duplicate link showed the inline error; re-entering a link loaded from cache; an invalid link restored presets; `video_unavailable` and `blocked` showed the reason with presets kept (Try again only for `blocked`); presets ⇄ formats toggle worked. Screenshots at 1200px and 375px (no horizontal overflow, `scrollWidth` 360). `node --check` passes. No Go files were edited.
- Blocker/notes: Needs Codex to implement `GET /api/inspect` and the `format` job field; until then the live page hides the picker (404) and uses presets. Real inspection is also subject to the YouTube rate limiting seen on this network.
- Next: Agree the inspect/format contract with Codex, then verify the picker against the Go server with a real video.

### 2026-09-26 — Claude: merged-app check and output-folder UI

- Result: Checked the merged `master`: `app.js`/`api.js` differ from `agent/claude-ui-health` only by Prettier formatting, and the Go job API matches the JSON shapes in `web/README.md`. On branch `agent/claude-post-merge`, added an output-folder row to the New download panel (view, Change, Save/Cancel, Escape to cancel, inline server errors), settings methods in both data clients, and removed UI error hints that repeated the server's own advice. Recorded two backend proposals in `web/README.md`: a distinct error code for YouTube bot checks and sentence-case validation messages.
- Verification: `go test ./...` passed on merged `master`. Ran the merged app with temp data/download dirs and used the Go-served page at 127.0.0.1:8799: health showed "Tools ready"; submitting `watch?v=jNQXAC9IVRw` (audio M4A) showed the job downloading, then failed in history with Retry. A `yt-dlp --simulate` run showed the cause: YouTube returned HTTP 429 and "Sign in to confirm you're not a bot" for this network. For settings, ran a temp build of Codex's uncommitted settings API behind a scratch proxy serving this branch's UI: the folder loaded, a relative path and a missing drive were rejected with the server's message, saving an existing folder updated the page and `GET /api/settings`, and Escape cancelled an edit. Fixture mode at 375px showed the inline error with no horizontal overflow. `node --check` passes. No Go files were edited.
- Blocker/notes: Real downloads cannot be verified from this network until YouTube stops rate-limiting it. The settings API was tested from Codex's uncommitted working tree; re-check after it is committed. Temporary preview files and servers were removed.
- Next: After Codex commits settings, merge this branch and re-check the Go-served page. Then switch active jobs from list polling to the per-job SSE stream (`/api/jobs/{id}/events`).

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
