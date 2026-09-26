# Web UI shell

A static, client-rendered page in `static/`: plain HTML, CSS, and ES modules with no build step, so the Go server can embed and serve the folder as-is.

| File                 | Purpose                                                   |
| -------------------- | --------------------------------------------------------- |
| `static/index.html`  | Page markup and the job row template                      |
| `static/app.css`     | Responsive layout, light/dark themes                      |
| `static/app.js`      | URL validation, rendering, form and job actions           |
| `static/api.js`      | Data clients: HTTP (default) and fixture (`?fixture=...`) |
| `static/fixtures.js` | Sample jobs and health responses                          |

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
| `/?fixture=loading`  | Loading skeletons that never resolve                                                               |
| `/?api=live`         | Same as `/`; calls the real `/api/...` routes                                                      |

## Proposed API shapes

The UI expects these shapes. They are a proposal for integration; the backend owner decides the final contract, and `api.js`/`app.js` will be adjusted to match.

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

Until per-job SSE exists, the HTTP client polls `GET /api/jobs` every 2 seconds. If the server answers a job route with 404 or 405 and no JSON error, the UI shows "Downloads aren't available yet" and stops polling.

## Output folder

The New download panel shows the current output folder from `GET /api/settings` (`{ "downloads_dir": "..." }`) with a Change button. Saving sends `PUT /api/settings` with the same field and shows the server's `invalid_directory` message under the input. The section stays hidden if the settings route is missing or the server is unreachable. Fixture mode keeps the folder in memory and only checks that the path looks absolute.

## Backend proposals from UI testing

- **Classify YouTube bot checks.** When YouTube answers `HTTP Error 429` or "Sign in to confirm you're not a bot", yt-dlp fails and the job currently gets `download_failed` with "Check that yt-dlp is up to date", which misleads. A distinct code (for example `blocked`) with a message such as "YouTube is limiting requests from this network. Wait a while, then retry." would be accurate. Cookie support stays deferred.
- **Sentence-case validation messages.** `invalid_directory` returns the raw Go error text ("choose an existing, writable absolute folder"); the UI shows server messages verbatim.

## Client-side URL checks

These are for fast feedback only; the server must validate again. Accepted: `http(s)` links on `youtube.com`, `www.`/`m.`/`music.youtube.com`, and `youtu.be` that identify one video (`watch?v=`, `youtu.be/<id>`, `/shorts/`, `/live/`, `/embed/`). A missing scheme gets `https://`. Playlist-only links are rejected with guidance; a video link that also carries `list=` is accepted with a note that only the video will be downloaded.
