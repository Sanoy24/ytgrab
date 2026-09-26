# Web UI shell

A static, client-rendered page in `static/`: plain HTML, CSS, and ES modules with no build step, so the Go server can embed and serve the folder as-is.

| File                 | Purpose                                                   |
| -------------------- | --------------------------------------------------------- |
| `static/index.html`  | Page markup and the job row template                      |
| `static/app.css`     | Responsive layout, light/dark themes                      |
| `static/app.js`      | URL validation, rendering, form and job actions           |
| `static/api.js`      | Data clients: HTTP (default) and fixture (`?fixture=...`) |
| `static/fixtures.js` | Sample jobs, health, and inspection responses |
| `static/formats.js` | Groups inspected formats into video/audio choices |

## Checking syntax

The scripts are ES modules. `node --check file.js` parses them as classic scripts and misses module-only errors such as a duplicate top-level `function`. Check a `.mjs` copy instead:

```powershell
Get-ChildItem web/static/*.js | ForEach-Object { Copy-Item $_ "$env:TEMP/$($_.BaseName).mjs"; node --check "$env:TEMP/$($_.BaseName).mjs" }
```

## Preview

Any static server works. From the repository root:

```powershell
python -m http.server 8765 --bind 127.0.0.1 --directory web/static
```

Then open `http://127.0.0.1:8765/`. Opening `index.html` from disk does not work because browsers block ES modules on `file://`.

Fixture scenarios are selected with a query parameter:

| URL                  | Shows                                                                                              |
| -------------------- | -------------------------------------------------------------------------------------------------- |
| `/`                  | Live app when served by Go; a standalone static preview cannot reach the Go API                    |
| `/?fixture=default`  | Active, queued, completed, failed, interrupted, and cancelled sample jobs, with simulated progress |
| `/?fixture=empty`    | Empty queue and history                                                                            |
| `/?fixture=error`    | Server unreachable: load error with retry, offline banner, failing submit                          |
| `/?fixture=degraded` | Missing `yt-dlp` and `ffmpeg`, with real health message text                                       |
| `/?fixture=loading` | Loading skeletons that never resolve (format check also never finishes) |
| `/?fixture=blocked` | Format check fails with a YouTube rate-limit message; presets remain |
| `/?fixture=expired` | The first format job gets `inspection_required`; the page re-checks and retries once |
| `/?api=live`         | Same as `/`; calls the real `/api/...` routes                                                      |

## API shapes

The Go backend and UI use these shapes.

`GET /api/jobs` returns `{ "jobs": [Job] }` (a bare array is also accepted). `POST /api/jobs` accepts `{ "url": string, "preset": Preset }` and returns the created `Job`.

```jsonc
// Job
{
  "id": "job_7",
  "url": "https://www.youtube.com/watch?v=…",
  "video_id": "dQw4w9WgXcQ", // nullable
  "title": "…", // nullable; the UI falls back to video_id, then URL
  "preset": "video-best", // video-best | video-1080 | video-720 | audio-m4a | audio-mp3
  "state": "downloading", // queued | inspecting | downloading | processing | completed | failed | cancelled
  "attempt": 1,
  "progress": {
    // nullable
    "downloaded_bytes": 187000000,
    "total_bytes": 412000000, // nullable -> indeterminate bar
    "speed_bps": 6400000, // nullable
    "eta_seconds": 35, // nullable
  },
  "output_path": null, // set once completed
  "error": { "code": "interrupted", "message": "…" }, // nullable
  "created_at": "2026-09-26T12:00:00Z",
  "updated_at": "2026-09-26T12:02:00Z",
}
```

Errors: non-2xx responses with `{ "error": { "code": string, "message": string } }`. The UI shows `message` to the user and gives extra hints for the codes `video_unavailable`, `network`, and `dependency_missing`. `invalid_url` and `duplicate_job` are shown next to the URL field.

`GET /api/system/health` returns:

```jsonc
{
  "status": "ready", // ready | degraded
  "dependencies": [
    {
      "name": "ffmpeg",
      "available": false,
      "required": true,
      "message": "Install ffmpeg and add it to PATH or the tools directory.",
    },
  ],
}
```

The live client polls `GET /api/jobs` every 2 seconds for queue/history changes and opens `GET /api/jobs/{id}/events` for each running job. The SSE route sends a job snapshot when connected and after each persisted update, then closes when the job reaches a terminal state.
The tools panel lists every dependency with a short version (`ffmpeg version 8.0.1-full_build…` shows as `8.0.1`), its message when it carries advice, and the report's `note`. It opens automatically when a required tool is missing; the header pill toggles it.

`GET /api/settings` returns `{ "downloads_dir": string }`; `PUT /api/settings` accepts the same shape. The folder must already exist, be writable, and be an absolute path. Changing it affects jobs that start afterward. If the server answers a job route with 404 or 405 and no JSON error, the UI shows "Downloads aren't available yet" and stops polling.

## Playlists

A playlist link (`/playlist?list=…`) opens a review list instead of the format picker: the playlist title, up to 50 entries with durations, all ticked, and a note for private or deleted entries and for playlists longer than 50. The quick presets apply to every video; the Add button reads "Add N videos" and is the confirmation. Pressing Add before the list loads only opens the review. A video link that also carries `list=` downloads just that video and offers "Download the whole playlist instead". Editing the link leaves playlist mode. Mix links (`list=RD…`) are refused.

```jsonc
// GET /api/playlist?url=...
{ "id": "PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2", "title": "Project Gold",
  "entries": [{ "video_id": "nV_awXI9XJY", "title": "…", "duration_seconds": 261 }],
  "total": 7, "truncated": false, "unavailable": 0 }

// POST /api/playlist/jobs   ->  201 { "jobs": [Job], "skipped": 1 }
{ "video_ids": ["nV_awXI9XJY", "M788vUWI2Rk"], "preset": "audio-m4a" }
```

Errors: `invalid_url`, `mix_playlist`, `video_unavailable` ("This playlist is unavailable or private."), `blocked`, `invalid_request` (bad or more than 50 IDs; nothing is created), `invalid_preset`. Duplicates and videos already queued count as `skipped`. Jobs get the listed title right away.

## Output folder

`GET /api/settings` returns `{ downloads_dir, configured, default_dir, can_pick }`. While `configured` is false, a card at the top of New download asks for a folder and Add is refused until one is chosen: **Choose folder…** calls `POST /api/settings/pick-folder`, which opens the operating system's folder window on this computer and returns the new settings, `{ ...settings, cancelled: true }`, or `picker_unavailable` (the page then shows the typed-path form); **Use …** calls `POST /api/settings/use-default`. Afterwards **Change…** opens the window again and **Type a path** shows the form. Fixture modes: `?fixture=first-run` (first pick is cancelled, the second succeeds) and `?fixture=no-picker`.

The New download panel shows the current output folder from `GET /api/settings` (`{ "downloads_dir": "..." }`) with a Change button. Saving sends `PUT /api/settings` with the same field and shows the server's `invalid_directory` message under the input. The section stays hidden if the settings route is missing or the server is unreachable. Fixture mode keeps the folder in memory and only checks that the path looks absolute.

## Backend behavior from UI testing

- YouTube HTTP 429 and bot-confirmation failures return `blocked` with a wait-and-retry message. Cookie support stays deferred.
- `invalid_directory` returns a sentence-case message because the UI displays server errors verbatim.

## Format picker

When a valid video link is pasted (or typed, after a 500 ms pause), the UI calls `GET /api/inspect` and replaces the quick presets with the video's real formats: one video row per resolution and frame rate (H.264/MP4 preferred when several codecs exist, size estimated with the best M4A audio), every distinct audio stream, and an "MP3 · converted" row. A preset chosen before the list arrives carries over to the closest real format. Results are cached per link for the page session; stale requests are aborted. On failure the presets stay, with the reason and a Try again button (not offered for `video_unavailable`). A 404 from the route hides the feature silently. Grouping logic lives in `static/formats.js`. YouTube's `-drc` (dynamic range compressed) audio copies are hidden when the original stream is listed.

The page trusts a cached inspection for 9 minutes, just under the server's 10. If job creation still returns `inspection_required`, it re-checks the link and retries once with the same choice; if that choice is gone, it asks the user to pick again.

**Suggestion for the server:** a picked H.264 video is merged with the best audio, which is usually Opus, so the result is `.mkv`. Selecting `ID+bestaudio[ext=m4a]/ID+bestaudio` for `avc1` video would produce an `.mp4` that plays everywhere.

`GET /api/inspect?url=...` response (field names follow yt-dlp's info JSON, filtered to video-only and audio-only streams):

```jsonc
{
  "video_id": "dQw4w9WgXcQ",
  "title": "…",
  "duration_seconds": 1325,             // nullable
  "video": [                            // video-only streams (vcodec != none, acodec == none)
    { "format_id": "137", "ext": "mp4", "height": 1080, "width": 1920, "fps": 30,
      "vcodec": "avc1.640028", "filesize": null, "filesize_approx": 278000000 }
  ],
  "audio": [                            // audio-only streams
    { "format_id": "140", "ext": "m4a", "acodec": "mp4a.40.2", "abr": 129.5,
      "filesize": 21400000, "filesize_approx": null, "language": "en" }
  ]
}
```

Errors use the usual `{ "error": { "code", "message" } }`: `invalid_url`, `video_unavailable`, `blocked` (YouTube rate limit or bot check), `network`, `dependency_missing`.

Creating a job from a picked format sends `format` instead of `preset`:

```jsonc
{ "url": "https://…", "format": { "kind": "video", "id": "137" } }  // server downloads "137+bestaudio/137", merges to mp4/mkv
{ "url": "https://…", "format": { "kind": "audio", "id": "140" } }  // server downloads "140" as-is
```

The server validates `id` with `^[0-9A-Za-z_-]{1,32}$` and against a recent cached inspection, then builds the selector itself; the raw value never becomes a free-form yt-dlp argument. The job returns `"preset": null` and `"format": { "kind", "id", "label" }`, where `label` is a server-made display string such as `"Video · 1080p"`; the UI shows it in job rows. An expired inspection returns `inspection_required` so the page can check again.



These are for fast feedback only; the server must validate again. Accepted: `http(s)` links on `youtube.com`, `www.`/`m.`/`music.youtube.com`, and `youtu.be` that identify one video (`watch?v=`, `youtu.be/<id>`, `/shorts/`, `/live/`, `/embed/`). A missing scheme gets `https://`. Playlist-only links are rejected with guidance; a video link that also carries `list=` is accepted with a note that only the video will be downloaded.
