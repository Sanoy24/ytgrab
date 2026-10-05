# Architecture

YTGrab is a single Go program that serves a browser UI on `127.0.0.1` and orchestrates [yt-dlp](https://github.com/yt-dlp/yt-dlp) and [FFmpeg](https://ffmpeg.org/). It is built for one local user: no accounts, no remote access, no cloud storage.

## Responsibilities

| Component | Owns |
| --- | --- |
| Browser UI (`web/static`) | Download, Library, and Settings views: link entry, format and playlist review, queue and history, settings. Plain HTML, CSS, and ES modules embedded in the binary; no build step. |
| Go server (`internal/api`) | Input validation, the JSON API, progress streams (SSE), local-only request checks. |
| Queue (`internal/queue`) | A bounded pool of workers (two by default), priority order, pause and cancellation, and automatic retry of network failures. |
| Desktop (`internal/tray`, `internal/autostart`, `internal/activity`) | The tray or menu-bar icon and its menu, download notifications, and starting at sign-in. |
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
internal/activity/           queue summaries for the tray (counts, newly finished downloads)
internal/app/                startup, shutdown, wiring, doctor checks, update checks
internal/app/deps/           finding tools and reading their versions
internal/api/                HTTP routes, SSE, request protection
internal/autostart/          start at sign-in (Run key, XDG autostart, Login Agent)
internal/config/             environment configuration
internal/cooldown/           the shared pause when YouTube limits the network
internal/diskspace/          free-space checks before a download starts
internal/domain/             jobs, states, presets, sections, folder names, URL parsing
internal/downloader/ytdlp/   yt-dlp process adapter, inspection, playlists
internal/picker/             the operating system's folder window
internal/process/            cross-platform process-tree control
internal/queue/              scheduling, pause, cancellation, retries
internal/reveal/             show a file in its folder, open a file or folder
internal/selfupdate/         one-click updates: download, check, and swap the program
internal/settings/           user settings (folder, parallel downloads, format, speed, subtitles, …)
internal/setup/              "ytgrab setup": tool installation
internal/store/sqlite/       persistence and migrations
internal/tray/               tray / menu-bar icon, menu, notifications
internal/trayhost/           whether this desktop can show a tray icon
internal/watch/              watched channels and playlists, and their checks
web/static/                  the browser UI: app.js (download form, queue, library, settings),
                             watching.js, search.js, ui.js (shared helpers), api.js (server
                             and sample-data clients), formats.js, links.js
```

## Download flow

1. The page submits a link with either a preset (`video-best`, `video-1080`, `video-720`, `audio-m4a`, `audio-mp3`, `audio-opus`, `audio-flac`, `audio-wav`) or a format picked from an inspection, and optionally a section (start and end seconds) to download only part of the video.
2. The server validates the link (YouTube hosts only, one video ID), stores a `queued` job, and returns it.
3. A worker starts yt-dlp with an explicit argument list — never a shell string — and the URL as the final argument after `--`.
4. yt-dlp prints progress through a custom `--progress-template`; the adapter parses it and updates the job. The page receives updates over Server-Sent Events.
5. The job completes only after yt-dlp exits successfully and the reported output file is confirmed to exist inside the output folder.

Output files are named `Title [video-id] 1080p.ext` (audio: `… 130k.ext`; a section adds its range, `… 720p 1m05s-2m30s.ext`), so different qualities and clips of one video never collide. Playlist downloads can go into a subfolder named after the playlist; the name is made safe for every system (`domain.SafeFolderName`) when the job is created and checked again before downloading. Subtitles (embedded, or saved as `.srt`) and the speed limit are settings applied to each download as yt-dlp options built from fixed lists of values. A picked video format is paired with audio from the same container family (MP4 with M4A, WebM with WebM) so merges stay lossless.

### Job states

`queued → downloading → processing → completed`, with `failed` and `cancelled` reachable from any active state. `paused` is reachable from any active state and leads back to `queued` (resume) or to `cancelled`; a paused job keeps its place and its partial file, is never started by workers, and survives restarts. Waiting jobs start in `priority` order (raised by "move to top"), then oldest first. Transitions are enforced in one place (`internal/domain`). Retry keeps the job ID, increments `attempt`, clears progress and errors, and returns the job to `queued`; completed jobs cannot be retried.

Jobs are stored as a JSON payload with indexed ID, state, video ID, and creation time. A partial unique index prevents two active (including paused) jobs for the same video, and updates compare the previous state atomically. On startup, jobs left active by a closed app become `failed` with the `interrupted` code; Retry resumes them from yt-dlp's partial file.

## Format inspection

When a valid link is entered, the page asks the server to inspect it (`yt-dlp --dump-json`) and replaces the presets with the real options: one row per resolution and frame rate, and each audio stream. Inspection never blocks a download — presets stay usable and remain the fallback. The server serializes inspections, spaces them at least two seconds apart, and caches up to 64 results for ten minutes. A job created from an inspected format sends only a format kind and ID; the server checks it against its cached inspection and builds the yt-dlp format selector itself.

## Playlists

A playlist link is listed 50 entries at a time, each page one `--flat-playlist --playlist-items <start>:<end>` request sharing the inspection throttle. The user reviews the list and confirms with a button that states the count. The server validates every video ID before creating any job, builds each URL itself, and skips videos that are already queued. YouTube Mixes are refused because they are generated endlessly.

## Settings and the folder window

The output folder is stored in SQLite. On first run no folder is set and the page asks for one before downloading. Because browsers cannot reveal real folder paths, the server opens the operating system's own folder window on the user's desktop: Explorer's folder dialog on Windows (via PowerShell), `choose folder` on macOS, and zenity or kdialog on Linux. The dialog scripts are constants; the starting folder is passed through an environment variable. Typing a path is always available as a fallback.

## Security

- The server binds to loopback only and rejects requests whose `Host` is not a loopback name, which blocks DNS-rebinding pages.
- Phone access (`internal/phone`, off by default) adds a second server on the local network, one port up. It answers only private addresses and IP-address hosts, and only phones paired with a one-time QR code (10 minutes, single use). A phone's key is 256 random bits in an `HttpOnly` cookie, stored as a SHA-256 hash; removing the phone or turning access off revokes it. An allowlist limits phones to downloads, Watching, and the Library: settings, files on the computer, sign-in, backups, and updates stay computer-only, and new routes are refused until listed. The connection is plain HTTP, so it is meant for a trusted home network.
- Cross-site writes are rejected using `Origin` and `Sec-Fetch-Site`.
- Every response carries a Content-Security-Policy that allows only the page's own files and the thumbnail servers of YouTube (`i.ytimg.com`), X (`pbs.twimg.com`), Reddit (`external-preview.redd.it`, `preview.redd.it`), Instagram (`*.fbcdn.net`, `*.cdninstagram.com`), and Vimeo (`i.vimeocdn.com`) and forbids framing (`frame-ancestors 'none'`, plus `X-Frame-Options: DENY`). A framed copy of the page would make same-site requests, so this is what stops another site from disguising YTGrab's buttons under its own (clickjacking). The page builds its content with text nodes, never HTML from YouTube data.
- Search words, watch links, and the Send to YTGrab link are data only: search words are one argument after `--`, watch links are rebuilt from validated parts, and a handed-over link only fills the link field. Opening a file uses the same ownership check as deleting it.
- Links are validated before use; user input never becomes a yt-dlp option. Format IDs are checked against a strict pattern and a recent inspection.
- Output paths reported by yt-dlp must resolve inside the output folder.
- Request bodies and retained process output are size-limited.
- Browser sign-in is opt-in. The setting accepts only names from a fixed browser list and becomes yt-dlp's `--cookies-from-browser <name>`; yt-dlp reads the cookies itself on each run, and YTGrab never reads, stores, or logs them.

## When YouTube limits the network

YouTube sometimes answers with HTTP 429 or a "confirm you're not a bot" check. Retrying straight away tends to extend the block, so a shared gate (`internal/cooldown`) pauses the download queue and format and playlist checks together: 15 minutes for the first block, then 30 and 60 while blocks continue, reset by the next successful download. A blocked job returns to the queue and runs when the pause ends, up to three attempts. Format checks during a pause answer immediately without contacting YouTube, and the job list reports `paused_until` so the page can show a countdown; `POST /api/system/resume` ends a pause early. To look less like a burst, consecutive downloads start 3–8 seconds apart and yt-dlp waits half a second between requests (`--sleep-requests`). The pause is kept in memory only. It is YouTube's alone: X posts skip the gate and the inspection throttle's block tracking, and X's own limits fail with `x_limited` instead of `blocked`, so a busy X post never stops YouTube downloads.

X posts reuse the same adapter. `domain.ParseVideoURL` rewrites any `x.com`/`twitter.com` post link to `https://x.com/i/status/<id>` and the job records `site: "x"`; `buildArgs` then picks X's MP4s, which already contain sound, with `-f b` and `-S res:N` (the shorter side, for portrait videos), decodes the HTML entities X leaves in titles, and skips YouTube-only options (subtitles, chapters, SponsorBlock). Reddit posts follow the YouTube download path (separate video and audio streams, merged), but with the post's ID written into file names and sizes estimated from bitrate; `parsePostInspection` reads them. Vimeo links are rewritten to `player.vimeo.com/video/<id>` (with `?h=` for unlisted videos): vimeo.com's own pages make yt-dlp sign in, the embedded player doesn't. Vimeo's audio streams don't name their codec, so they are offered as the M4A they are saved as, and video is merged into MP4 like Instagram's. A download that fails for good (copy-protected or unavailable) has the cover image yt-dlp fetched removed. Instagram posts take the same path, picking a carousel's video with `--playlist-items` like X and merging into MP4 (yt-dlp would choose MKV for its VP9 with AAC). Preview images are kept only from `pbs.twimg.com`, `external-preview.redd.it`, `preview.redd.it`, and hosts under `fbcdn.net` and `cdninstagram.com`, the other image hosts the Content-Security-Policy allows.

## One copy at a time

On startup YTGrab claims its port before opening the database. If the port is taken by a running YTGrab (recognized by the `X-YTGrab-Version` response header), the new launch opens the browser to it and exits; it never runs startup recovery or starts workers against the running copy's jobs. An exclusive lock on `ytgrab.lock` in the data folder (`LockFileEx` on Windows, `flock` elsewhere) also stops a second copy on a different port from sharing the database. The lock is released automatically if the process ends.

## On the desktop

Started from a shortcut on Windows (detected by owning its console), YTGrab starts itself again without a console window and writes its log to `ytgrab.log` in the data folder; startup errors are shown in a message box. Where a tray exists (Windows always; Linux with a StatusNotifierWatcher such as KDE, Ubuntu, or GNOME with AppIndicator; macOS builds made with cgo) it shows an icon with Open, Open downloads folder, Start at login, and Quit, using `fyne.io/systray`. Quit cancels the server's context, the same as Ctrl+C. `YTGRAB_NO_TRAY=1` turns the icon off.

`internal/activity` polls the job list every two seconds and reports counts for the icon's tooltip and the downloads that just finished or failed, which become system notifications (from the tray icon on Windows, `org.freedesktop.Notifications` on Linux, `osascript` on macOS). While a YTGrab page is open, recognized by its open progress streams, the tray stays quiet so the page's own notifications aren't doubled.

Start at login writes a per-user entry that runs YTGrab without `--open`, so it waits in the tray: the `Run` registry key on Windows, `~/.config/autostart/ytgrab.desktop` on Linux, and a Login Agent plist on macOS. Each start re-points an existing entry at the running program, so moving it or a package-manager update doesn't break it.

The page can also be reached through the **Send to YTGrab** bookmark, which opens `/?url=<the YouTube page>`. The page only puts that link in the field; nothing is queued until the user presses Add, so another site can't queue downloads this way.

## Watched channels and playlists

`internal/watch` follows channels (their uploads tab) and playlists. A watch stores its canonical URL, format, and folder choice; `watch_seen` records every video ID it has seen. Adding a watch lists the source once and marks everything listed as seen, so only videos that appear later are downloaded (plus an optional backfill of the newest 0–10). A background loop wakes every 15 minutes and checks, one at a time, the unpaused watches whose interval (1, 6, or 24 hours) has passed: it lists the newest 30 uploads (or up to 500 playlist entries) with `--flat-playlist`, through the inspector's throttle and the YouTube pause, and queues unseen videos oldest first, at most 25 per check. Entries yt-dlp lists as upcoming or live are ignored (not marked seen) until they can be downloaded; unseen videos that fail the watch's filters (minimum length, title words) are marked seen without downloading. A check that YouTube blocks stops the round and isn't recorded against the watch; other failures are shown on it. Jobs are created like playlist jobs (server-built URLs, `SafeFolderName` folders), so the rest of the pipeline is unchanged.

## Processes, cancellation, and shutdown

Child processes run with a context. Cancellation stops the whole process tree (`taskkill /T` on Windows, a process group on Unix), and yt-dlp keeps its `.part` file so a retry resumes. On Ctrl+C or SIGTERM the server stops accepting requests, closes open progress streams, and cancels running jobs; they are marked `interrupted` on the next start.

## Tools and releases

Tools are found in this order: `YTGRAB_TOOLS_DIR`, a `tools` folder beside the program, a `tools` folder in the working directory, WinGet's `Links` folder on Windows, then `PATH`. Version checks allow 15 seconds (the Windows yt-dlp build unpacks itself on every run) and are cached per file.

yt-dlp is the part most likely to need updating, because YouTube changes often. The server looks up the latest yt-dlp release at most once a day (a single redirect request, in the background) and marks the installed one as outdated when a newer release exists. The tools panel and `ytgrab doctor` then offer an update, which installs the new version into YTGrab's own tools folder — never over a system-wide yt-dlp — with the same checksum verification as setup, and is refused while downloads are running.

Unless turned off in Settings, a background loop updates yt-dlp by itself: two minutes after start and then every 30 minutes, when nothing is downloading and a newer release is known, it runs the same checksum-verified update.

YTGrab's own releases are checked the same way (GitHub's `releases/latest` redirect, at most once a day, never for development builds). `GET /api/system/version` reports the result with the update command for how this copy was installed, recognized from its path (Scoop, Homebrew, the curl installer, `go install`); copies unpacked from a zip get a link to the release instead.

`ytgrab doctor` reports tools, folders, and the port. `ytgrab setup` installs what is missing after asking: yt-dlp from its official GitHub release, verified against the published SHA-256 checksums; FFmpeg and Deno through winget or Homebrew; package-manager commands are printed on Linux.

`scripts/package.ps1` builds release archives. The release workflow builds the Windows and Linux archives on Windows and the macOS archives on a Mac (the menu-bar icon needs cgo), publishes them together, updates the Homebrew tap and Scoop bucket, and then installs the release with each method in CI to check it starts. The Windows zip bundles a checksum-verified `yt-dlp.exe`; every archive includes `manifest.json` (versions and hashes) and `SHA256SUMS`. FFmpeg and a JavaScript runtime are installed separately because of their size and licensing.

## Updating itself

`internal/selfupdate` replaces a copy unpacked from a release archive or installed by `install.sh` (Windows x64 and Linux; not Scoop, Homebrew, `go install`, or macOS, which is untested). It downloads `ytgrab-<version>-<os>-<arch>` from the GitHub release with its `.sha256` file and refuses any mismatch, takes only the program from the top of the archive, and runs it with `--version`, which must print the expected version. The program then takes the old one's place: on Windows the running `ytgrab.exe` can only be renamed, so it becomes `ytgrab.exe.old`, which the next start deletes. The handler answers, then ends `app.Run` with `ErrRestart`; by then the port and data folder are free, and `main` starts the new program in the background like a shortcut start. Updates are refused while downloads run.

## Browser extension

`extension/` is a Manifest V3 extension for Chrome, Edge, and Firefox. Its toolbar button and context menu open `http://127.0.0.1:<port>/?url=<address>`, the same entry point as the Send to YTGrab bookmark, reusing an open YTGrab tab. It first checks `GET /api/system/version` for the `X-YTGrab-Version` header, so it never sends links to another program on the port; it makes no other requests, and YTGrab's origin check would refuse any change it tried to make. Its permissions are `activeTab` (the clicked tab's address), `contextMenus`, `storage` (the port), and host access to `127.0.0.1`. The release job stamps the version into the manifest and publishes `ytgrab-extension-<version>.zip`.

## Defaults

| Setting | Value |
| --- | --- |
| Concurrent downloads | 2 |
| Fragments per download | 4 |
| Automatic retries | network failures, up to 3 attempts |
| YouTube block pauses | 15, 30, then 60 minutes; up to 3 attempts per job |
| Spacing between download starts | 3–8 seconds |
| Inspection cache | 64 entries, 10 minutes |
| Playlist page size | 50 videos |
| Videos per playlist confirmation | 200 |
| Shutdown timeout | 5 seconds |
| Speed limit | none |
| Subtitles | off |
| Automatic yt-dlp updates | on; checked every 30 minutes |
| YTGrab and yt-dlp release checks | at most once a day |
