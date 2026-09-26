# Web UI shell

A static, client-rendered page in `static/`: plain HTML, CSS, and ES modules with no build step, so the Go server can embed and serve the folder as-is.

| File | Purpose |
| --- | --- |
| `static/index.html` | Page markup and the job row template |
| `static/app.css` | Responsive layout, light/dark themes |
| `static/app.js` | URL validation, rendering, form and job actions |
| `static/api.js` | Data clients: fixture (default) and HTTP (`?api=live`) |
| `static/fixtures.js` | Sample jobs and health responses |

## Preview

Any static server works. From the repository root:

```powershell
python -m http.server 8765 --bind 127.0.0.1 --directory web/static
```

Then open `http://127.0.0.1:8765/`. Opening `index.html` from disk does not work because browsers block ES modules on `file://`.

Fixture scenarios are selected with a query parameter:

| URL | Shows |
| --- | --- |
| `/` | Active, queued, completed, failed, interrupted, and cancelled jobs, with simulated progress |
| `/?fixture=empty` | Empty queue and history |
| `/?fixture=error` | Server unreachable: load error with retry, offline banner, failing submit |
| `/?fixture=degraded` | Missing `ffmpeg`/`ffprobe` and optional JavaScript runtime |
| `/?fixture=loading` | Loading skeletons that never resolve |
| `/?api=live` | Calls the real `/api/...` routes (not yet tested against the Go server) |

## Proposed API shapes

The UI expects these shapes. They are a proposal for integration; the backend owner decides the final contract, and `api.js`/`app.js` will be adjusted to match.

`GET /api/jobs` returns `{ "jobs": [Job] }` (a bare array is also accepted). `POST /api/jobs` accepts `{ "url": string, "preset": Preset }` and returns the created `Job`.

```jsonc
// Job
{
  "id": "job_7",
  "url": "https://www.youtube.com/watch?v=…",
  "video_id": "dQw4w9WgXcQ",          // nullable
  "title": "…",                       // nullable; the UI falls back to video_id, then URL
  "preset": "video-best",             // video-best | video-1080 | video-720 | audio-m4a | audio-mp3
  "state": "downloading",             // queued | inspecting | downloading | processing | completed | failed | cancelled
  "attempt": 1,
  "progress": {                       // nullable
    "downloaded_bytes": 187000000,
    "total_bytes": 412000000,         // nullable -> indeterminate bar
    "speed_bps": 6400000,             // nullable
    "eta_seconds": 35                 // nullable
  },
  "output_path": null,                // set once completed
  "error": { "code": "interrupted", "message": "…" }, // nullable
  "created_at": "2026-09-26T12:00:00Z",
  "updated_at": "2026-09-26T12:02:00Z"
}
```

Errors: non-2xx responses with `{ "error": { "code": string, "message": string } }`. The UI shows `message` to the user and gives extra hints for the codes `video_unavailable`, `network`, and `dependency_missing`. `invalid_url` and `duplicate_job` are shown next to the URL field.

`GET /api/system/health` is expected to return:

```jsonc
{
  "status": "ok",                     // ok | degraded | error
  "dependencies": [
    { "name": "ffmpeg", "found": false, "version": null, "required": true, "message": "Not found in the tools folder or PATH." }
  ]
}
```

Until per-job SSE exists, the HTTP client polls `GET /api/jobs` every 2 seconds.

## Client-side URL checks

These are for fast feedback only; the server must validate again. Accepted: `http(s)` links on `youtube.com`, `www.`/`m.`/`music.youtube.com`, and `youtu.be` that identify one video (`watch?v=`, `youtu.be/<id>`, `/shorts/`, `/live/`, `/embed/`). A missing scheme gets `https://`. Playlist-only links are rejected with guidance; a video link that also carries `list=` is accepted with a note that only the video will be downloaded.
