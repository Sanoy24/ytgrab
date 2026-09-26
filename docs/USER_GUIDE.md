# YTGrab user guide

YTGrab downloads YouTube videos and audio you are allowed to save, from a page in your browser. It runs only on your computer: the page is served from `127.0.0.1`, and files are written straight to your downloads folder by `yt-dlp`.

## Install (Windows release)

1. Unzip `ytgrab-<version>-windows-amd64.zip` into a folder you keep, for example `C:\Apps\YTGrab`. Don't run it from inside the zip.
2. Install ffmpeg, which merges video with audio and converts MP3:
   ```powershell
   winget install Gyan.FFmpeg
   ```
3. Install a JavaScript runtime, which YouTube extraction needs for many videos (either one):
   ```powershell
   winget install DenoLand.Deno
   winget install OpenJS.NodeJS.LTS
   ```
4. Double-click **Start YTGrab.cmd**. A console window shows the address and your browser opens `http://127.0.0.1:8787/`. Keep the window open while you use the app; close it or press Ctrl+C to stop.

`yt-dlp.exe` is included in the `tools` folder and was checked against the official yt-dlp checksums when the release was built (see `manifest.json`). Open a new console window after installing tools so YTGrab sees the updated `PATH`.

The header pill shows **Tools ready** when everything is found. Click it to see each tool, its version, and advice for anything missing.

### Linux and macOS

Release archives for Linux and macOS contain only the `ytgrab` program. Install `yt-dlp`, `ffmpeg`, and Deno or Node with your package manager (for example `brew install yt-dlp ffmpeg deno`), make the program executable with `chmod +x ytgrab`, and run `./ytgrab --open`. You can also put tools in a `tools` folder next to `ytgrab`.

## Download a video

1. Paste a video link (`youtube.com/watch?v=…`, `youtu.be/…`, or a Shorts link) into **Video link**.
2. YTGrab checks the video's formats. While it does, you can already use a quick preset:
   - **Best quality**, **Up to 1080p**, **Up to 720p**: video with audio.
   - **M4A**: original audio, no re-encoding. **MP3**: converted, uses more CPU.
3. When the check finishes, the presets are replaced by the real options: one row per resolution with its codec and estimated size, and each audio stream. H.264 picks are saved as `.mp4`. Use **Use standard presets instead** to switch back.
4. Press **Add to queue**. The queue shows live progress; merged downloads show the video part, then the audio part.

Files are named `Title [video-id] 1080p.mp4` (audio: `Title [video-id] 130k.m4a`), so different qualities of one video never overwrite each other.

## Download a playlist

Paste a playlist link (`youtube.com/playlist?list=…`). YTGrab lists up to 50 videos, all ticked. Untick what you don't want, choose a preset (it applies to every video), and press **Add N videos**. Nothing is queued until you press that button. Private and deleted videos are skipped. For a link to one video inside a playlist, YTGrab downloads just that video and offers **Download the whole playlist instead**. YouTube Mixes (auto-generated radio lists) can't be downloaded as playlists.

## Queue and history

- **Cancel** stops a download and keeps its partial file. **Retry** resumes from that partial file.
- If YTGrab closes while downloading, the job appears as failed with "The app closed while this download was running"; **Retry** resumes it.
- Network hiccups are retried automatically up to three attempts.
- **Copy path** copies a finished file's full path.

## Settings and data

- **Save to** (under the form) sets the output folder: any existing folder you can write to, given as a full path. It applies to downloads that start afterwards and is remembered.
- The default output folder is `Downloads\ytgrab` in your user folder.
- The job history is stored in `%AppData%\ytgrab\jobs.db` on Windows (`~/.config/ytgrab` on Linux, `~/Library/Application Support/ytgrab` on macOS).

Optional environment variables, set before starting:

| Variable | Purpose |
| --- | --- |
| `YTGRAB_LISTEN_ADDR` | Loopback address and port, default `127.0.0.1:8787`. Only `127.0.0.1` or `::1` are accepted. |
| `YTGRAB_TOOLS_DIR` | Extra folder searched first for `yt-dlp`, `ffmpeg`, `ffprobe`, `deno`, `node`. |
| `YTGRAB_DATA_DIR` | Folder for the job database. |
| `YTGRAB_DOWNLOAD_DIR` | Initial output folder when none has been saved in the page. |

Command-line flags: `--open` opens the browser after starting; `--version` prints the version.

## Troubleshooting

| What you see | What to do |
| --- | --- |
| **Tools missing** in the header | Click it. Install what it lists (see Install), then restart YTGrab from a new window. |
| "YouTube is limiting requests from this network. Wait a while, then retry." | YouTube is rate-limiting or asking for a bot check. Wait (often 15–60 minutes) and press Retry. Avoid checking many links quickly. Signed-in downloads (cookies) are not supported in this version. |
| "yt-dlp could not download this video. Check that yt-dlp is up to date and retry." | YouTube changes often. Update yt-dlp: `tools\yt-dlp.exe -U` in the YTGrab folder (or your package manager), then Retry. |
| "This video is unavailable or private." | The video is private, removed, age- or region-restricted. Check the link in a browser. |
| "Couldn't load formats" | The same causes as above; the quick presets still work. |
| Video saved as `.mkv` or `.webm` | Best quality often uses VP9 or AV1 video, which isn't stored in MP4. Pick an H.264 row or **Up to 1080p** for `.mp4`. |
| Console shows `listen on 127.0.0.1:8787 … Only one usage of each socket address` | YTGrab is already running (open `http://127.0.0.1:8787/`), or another program uses the port. Set `YTGRAB_LISTEN_ADDR=127.0.0.1:8788` and start again. |
| The page says "The ytgrab server isn't responding" | The console window was closed. Start YTGrab again and reload the page. |
| Antivirus warns about `yt-dlp.exe` | It is a packaged Python program, which some scanners flag. Compare its SHA-256 with `manifest.json` and the official release. |
| "Choose an existing, writable absolute folder." | Enter a full path like `D:\Videos` for a folder that already exists. |
