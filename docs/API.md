# Local API

The browser UI talks to the server through this JSON API. It is served only on the loopback address and is intended for the bundled UI; there are no API keys or versioning guarantees.

Errors use a non-2xx status and `{ "error": { "code": "...", "message": "..." } }`. Messages are written for end users.

## Routes

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/api/system/health` | Tool status |
| `POST` | `/api/system/update-ytdlp` | Install or update yt-dlp in YTGrab's tools folder |
| `GET` | `/api/system/version` | `{ version, latest, update_available, update_command, can_update, release_url }`; `latest` is checked against GitHub every 6 hours, and again when the page asks and the last check is over an hour old; `?refresh=1` checks right away (at most every 30 seconds). `update_command` is empty for copies unpacked from a zip |
| `POST` | `/api/system/update-ytgrab` | Install the latest YTGrab over this copy when `GET /api/system/version` says `can_update`; answers `{ version, restarting: true }`, then YTGrab restarts. Errors: `no_update`, `downloads_running`, `update_busy`, `update_failed` |
| `POST` | `/api/system/resume` | End a pause after YouTube limited this network (`204`) |
| `GET` | `/api/jobs` | Recent jobs, `{ "jobs": [Job] }` |
| `POST` | `/api/jobs` | Create a job from a preset or an inspected format; optional `section: { start, end }` in seconds downloads only that part (at least 1 second, within 24 hours); optional `split_chapters: true` also saves each chapter as its own file (not with `section`) |
| `GET` | `/api/jobs/{id}` | One job |
| `GET` | `/api/jobs/{id}/events` | Progress stream (Server-Sent Events) |
| `POST` | `/api/jobs/{id}/cancel` | Cancel a queued, running, or paused job |
| `POST` | `/api/jobs/{id}/pause` | Stop a queued or running job but keep it in the queue as `paused` (its partial file is kept) |
| `POST` | `/api/jobs/{id}/resume` | Return a paused job to the queue; it continues from the partial file |
| `POST` | `/api/jobs/{id}/top` | Start a queued or paused job before the other waiting ones |
| `POST` | `/api/jobs/{id}/retry` | Retry a failed or cancelled job |
| `GET` | `/api/search?q=words` | `{ results: [{ video_id, title, channel, duration_seconds }] }`, up to 12 videos; `invalid_query` for fewer than 2 or more than 200 characters |
| `GET` | `/api/watches` | `{ watches: [Watch], interval_hours, max_backfill, max_per_check }` |
| `POST` | `/api/watches` | `{ url, preset, folder, backfill, min_minutes, keywords, interval_hours }` (minutes 0–600, keywords up to 200 characters, interval 1, 6, or 24): lists the channel or playlist, counts what's there as seen, queues the newest `backfill` (0–10) videos; `watch_exists` if it's already watched |
| `POST` | `/api/watches/{id}/check` | Check now; returns the watch with `last_new` |
| `PUT` | `/api/watches/{id}` | Any of `{ preset, folder, paused, min_minutes, keywords, interval_hours }` |
| `DELETE` | `/api/watches/{id}` | Stop watching (downloads are kept) |
| `POST` | `/api/jobs/{id}/open` | Open a finished download's own file with its default app (`204`; `file_missing` if it was moved) |
| `POST` | `/api/jobs/{id}/reveal` | Show a finished file in the system file manager (`204`; `file_missing` if it was moved) |
| `DELETE` | `/api/jobs/{id}` | Remove a finished, failed, or cancelled job from the history; `?delete_file=true` also deletes a finished job's file |
| `POST` | `/api/settings/cookies-file` | Body: a cookies.txt in Netscape format (up to 1 MB). Saves it in the data folder and sets sign-in to `file`; `invalid_cookie_file` otherwise. `PUT /api/settings/cookies` with any other value deletes it |
| `GET` | `/api/backup` | A backup file (`format: "ytgrab-backup"`, `version: 1`): `settings`, `watches: [{ watch, seen }]`, and the finished, failed, and cancelled `jobs` |
| `POST` | `/api/backup/restore` | Merge a backup: `{ watches_added, watches_existing, jobs_added, jobs_existing, jobs_invalid, settings_skipped }`; `invalid_backup` for other files or a newer format |
| `GET` | `/api/library/files` | `{ files: { [job id]: { bytes, missing } } }` for finished downloads: each file's size now, or `missing: true` when it was moved or deleted |
| `POST` | `/api/history/clear` | Remove all finished, failed, and cancelled jobs, `{ "removed": 3 }`; files are kept. `?delete_files=true` also deletes each finished job's file (same check as `delete_file`) and returns `{ "removed", "files_deleted", "kept" }`; `kept` jobs stay because their file couldn't be deleted |
| `GET` | `/api/inspect?url=…` | A video's formats, grouped into video and audio |
| `GET` | `/api/playlist?url=…&start=1` | One page of up to 50 playlist entries for review; `next` gives the following page's `start` |
| `POST` | `/api/playlist/jobs` | One preset job per confirmed video ID (`video_ids`) or link (`urls`: single YouTube videos or X posts); the page sends links for several pasted links and for the videos of an X post. Optional `folder` (for example the playlist title) saves into that subfolder, made safe with the same rules on every system |
| `GET` | `/api/settings` | Output-folder settings |
| `PUT` | `/api/settings` | Set the output folder to a typed path |
| `POST` | `/api/settings/pick-folder` | Open the folder window on this computer |
| `PUT` | `/api/settings/preferences` | Set `max_downloads` (1–4) and/or `default_preset`; applies immediately |
| `PUT` | `/api/settings/preferences` also accepts `normalize_audio` | `true` evens out the loudness of MP3, FLAC, and WAV downloads |
| `PUT` | `/api/settings/preferences` also accepts `download_window` | `""` (any time) or `"start-end"` in whole hours, like `"1-7"` or `"22-6"`; outside it, `GET /api/jobs` includes `window_opens` |
| `PUT` | `/api/settings/preferences` also accepts `sponsorblock` | `"off"`, `"mark"`, or `"remove"` (sponsor, self-promotion, and interaction segments) |
| `PUT` | `/api/settings/preferences` also accepts `file_names` | One of `file_name_styles` from `GET /api/settings` |
| `PUT` | `/api/settings/preferences` also accepts `speed_limit_kbps` | One of `speed_limits` from `GET /api/settings` (kB/s, `0` for no limit) |
| `PUT` | `/api/settings/subtitles` | `{ "mode": "off" \| "embed" \| "file", "lang": "en" }`; only codes in `subtitle_languages` (from `GET /api/settings`) are accepted |
| `PUT` | `/api/settings/startup` | Windows only: start YTGrab in the tray at sign-in (`{ "enabled": true }`) or stop (`false`). `GET /api/settings` includes `start_at_login` only where this is supported |
| `PUT` | `/api/settings/cookies` | Turn browser sign-in on (`{ "browser": "firefox" }`) or off (`""`); only listed browsers are accepted |
| `POST` | `/api/settings/use-default` | Create and use the suggested folder |

Write requests from another site, and any request whose `Host` is not a loopback name, are rejected with `403`.

While YouTube is limiting this network, `GET /api/jobs` also returns `"paused_until": "2026-10-01T13:26:20Z"`; queued jobs wait until then.

## Job

```jsonc
{
  "id": "job_a4d9…",
  "url": "https://www.youtube.com/watch?v=jNQXAC9IVRw",
  "video_id": "jNQXAC9IVRw",
  "title": "Me at the zoo",          // null until known
  "preset": "audio-m4a",             // or null when a format was picked
  "format": null,                    // { "kind": "video", "id": "137", "ext": "mp4", "label": "Video · 1080p" }
  "state": "downloading",            // queued | inspecting | downloading | processing | completed | failed | cancelled
  "attempt": 1,
  "progress": {                      // null before the download starts
    "downloaded_bytes": 187000000,
    "total_bytes": 412000000,        // null when unknown
    "speed_bps": 6400000,            // null when unknown
    "eta_seconds": 35,               // null when unknown
    "stream": "video"                // "video" or "audio" while a merged download fetches each part
  },
  "output_path": null,               // set when completed
  "site": "x",                       // only for X posts (omitted for YouTube); video_id is then the post's ID
  "thumbnail": "https://pbs.twimg.com/…", // X posts only: the preview image
  "error": null,                     // { "code": "interrupted", "message": "…" }
  "created_at": "2026-09-26T12:00:00Z",
  "updated_at": "2026-09-26T12:02:00Z"
}
```

Presets: `video-best`, `video-1080`, `video-720`, `audio-m4a`, `audio-mp3`.

## Creating jobs

```jsonc
// POST /api/jobs with a preset
{ "url": "https://youtu.be/jNQXAC9IVRw", "preset": "video-1080" }

// POST /api/jobs with a format from a recent inspection
{ "url": "https://youtu.be/jNQXAC9IVRw", "format": { "kind": "video", "id": "137" } }
```

Returns `201` with the job. Errors: `invalid_url`, `invalid_preset`, `invalid_format`, `duplicate_job` (the video is already queued), and `inspection_required` (the inspection expired; inspect again and retry).

## Progress stream

`GET /api/jobs/{id}/events` sends the job as `data: {…}` when connected and after each saved update, and closes when the job finishes. The UI opens a stream for each running job and refreshes the job list every 2 seconds while work is active.

## Inspection

```jsonc
// GET /api/inspect?url=https://youtu.be/jNQXAC9IVRw
{
  "video_id": "jNQXAC9IVRw",
  "title": "Me at the zoo",
  "duration_seconds": 19,
  "video": [
    { "format_id": "160", "ext": "mp4", "height": 144, "width": 192, "fps": 30,
      "vcodec": "avc1.4d400c", "filesize": null, "filesize_approx": 505000 }
  ],
  "audio": [
    { "format_id": "140", "ext": "m4a", "acodec": "mp4a.40.2", "abr": 129.8,
      "filesize": 309288, "filesize_approx": null, "language": "en" }
  ]
}
```

Field names follow yt-dlp's format info. Errors: `invalid_url`, `video_unavailable`, `blocked` (YouTube is rate-limiting or asking for a bot check), `network`, `dependency_missing`.

For an X post (`https://x.com/<user>/status/<id>`, also `twitter.com`), the result adds `"site": "x"` and `"thumbnail"`, `video_id` is the post's ID, and `video` lists the MP4 files, which already contain sound (`protocol: "https"`, `tbr`); `audio` is empty, and audio presets take the sound out of the best file. Sizes are estimated from the streamed bitrate. A post with several videos also lists them as `videos: [{ index, video_id, url, duration_seconds, thumbnail }]`; a `/video/N` link (`video_id` `<post id>-N`) inspects and downloads that video alone. X-specific errors: `signin_required` (turn on browser sign-in) and `x_limited` (X is rate-limiting; never pauses YouTube).

A Reddit post (`https://www.reddit.com/r/<sub>/comments/<id>/…`, `redd.it/<id>`, or `v.redd.it/<id>`) adds `"site": "reddit"` and `"thumbnail"`; `video_id` is the post's ID (or the v.redd.it video's), and `video` and `audio` are separate streams like YouTube's, with `filesize_approx` estimated from bitrate. Errors add `signin_required` and `reddit_limited`.

A Vimeo video (`vimeo.com/<id>`, `vimeo.com/<id>/<hash>`, channel and group links, or `player.vimeo.com/video/<id>`) is rewritten to the embedded player's link, which works without signing in; it adds `"site": "vimeo"` and `"thumbnail"`, and `audio` lists the AAC streams as `m4a`. Errors add `signin_required`, `vimeo_limited`, and, for any site, `drm_protected`.

An Instagram post (`https://www.instagram.com/reel/<code>/`, also `/p/`, `/reels/`, `/tv/`) adds `"site": "instagram"` and `"thumbnail"`; `video_id` is the post's code, and `video` and `audio` are separate streams (no sizes: Instagram gives no length). A post with several videos lists them in `videos` like X, with links ending in `?item=N` (YTGrab's own) and IDs `<code>.N`. Errors add `signin_required` and `instagram_limited`.

## Playlists

```jsonc
// GET /api/playlist?url=https://www.youtube.com/playlist?list=PL…
{
  "id": "PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2",
  "title": "Project Gold",
  "entries": [{ "video_id": "nV_awXI9XJY", "title": "…", "duration_seconds": 261 }],
  "total": 7,          // YouTube's count, when known
  "start": 1,          // position of the first entry on this page
  "next": null,        // start of the next page, or null on the last page
  "truncated": false,  // true when more entries follow
  "unavailable": 0     // private or deleted entries that were skipped
}

// POST /api/playlist/jobs  ->  201 { "jobs": [Job], "skipped": 0 }
{ "video_ids": ["nV_awXI9XJY", "M788vUWI2Rk"], "preset": "audio-m4a" }
```

All IDs are validated before any job is created. Videos already queued, and repeated IDs, count as `skipped`. Errors: `invalid_url`, `mix_playlist`, `video_unavailable`, `blocked`, `invalid_request`, `invalid_preset`.

## Settings

```jsonc
// GET /api/settings
{
  "downloads_dir": "C:\\Users\\me\\Downloads\\ytgrab",
  "configured": false,     // true once a folder was chosen or YTGRAB_DOWNLOAD_DIR is set
  "default_dir": "C:\\Users\\me\\Downloads\\ytgrab",
  "can_pick": true         // a folder window is available on this system
}
```

`PUT /api/settings` takes `{ "downloads_dir": "D:\\Videos" }`; the folder must be an existing, writable absolute path (`invalid_directory` otherwise). `POST /api/settings/pick-folder` waits for the user and returns the updated settings, the settings with `"cancelled": true`, or `picker_unavailable` / `picker_busy`. Changes apply to downloads that start afterwards.

## Health

```jsonc
// GET /api/system/health
{
  "status": "ready",       // or "degraded" when a required tool is missing
  "checked_at": "2026-09-26T20:45:03Z",
  "dependencies": [
    { "name": "yt-dlp", "available": true, "required": true, "version": "2026.08.19",
      "path": "C:\\Apps\\YTGrab\\tools\\yt-dlp.exe", "message": "Available." }
  ],
  "note": "Executable checks only. YouTube access and yt-dlp-ejs availability are not verified."
}
```

yt-dlp also carries `"outdated": true` and an explanatory `message` when a newer release exists. The server looks up the latest release at most once a day in the background; offline, only versions older than 90 days are flagged.

## Updating yt-dlp

`POST /api/system/update-ytdlp` checks the latest release and, unless it is already installed, downloads it into YTGrab's tools folder (searched before `PATH`), verified against the release's SHA-256 checksums. It returns `{ "version": "2026.09.30", "previous": "2026.08.19", "updated": true }`, or `"updated": false` when the installed version was already the latest. Errors: `downloads_running` (`409`, Windows can't replace a running program), `update_busy` (`409`), `update_failed` (`502`).
