# YTGrab user guide

YTGrab downloads YouTube videos and audio you are allowed to save, from a page in your browser. It runs only on your computer: the page is served from `127.0.0.1`, and files are written straight to your downloads folder by `yt-dlp`.

## Install (Windows release)

1. Unzip `ytgrab-<version>-windows-amd64.zip` into a folder you keep, for example `C:\Apps\YTGrab`. Don't run it from inside the zip.
2. Double-click **Start YTGrab.cmd**. The first time, it runs a quick check. If something is missing, it runs setup, which asks before each step:
   - **yt-dlp**: downloaded from its official GitHub release and installed only if its checksum matches. A checked copy is already in the `tools` folder.
   - **FFmpeg** (merges video and audio, converts MP3) and **Deno** (runs YouTube's JavaScript checks): installed with `winget`.
3. Your browser opens `http://127.0.0.1:8787/`. Choose where downloads should go: **Choose folder…** opens the normal Windows folder window, or use the suggested `Downloads\ytgrab`.

Keep the console window open while you use the app; close it or press Ctrl+C to stop. Next time, **Start YTGrab.cmd** opens the app straight away.

### Check or repair the setup

From the YTGrab folder:

```powershell
.\ytgrab.exe doctor           # what's installed, the data and download folders, the port
.\ytgrab.exe setup            # install what's missing, asking first
.\ytgrab.exe setup --update-ytdlp   # get the latest yt-dlp when YouTube downloads start failing
```

`doctor` exits with code 1 when something required is missing. `setup --yes` answers yes to every question.

The header pill in the page shows **Tools ready** when everything is found. Click it for each tool's version and advice.

### Linux and macOS

Release archives for Linux and macOS contain only the `ytgrab` program. Make it executable with `chmod +x ytgrab`, then run `./ytgrab setup`: it downloads the official yt-dlp into a `tools` folder next to the program, installs FFmpeg and Deno with Homebrew on macOS, and prints the package-manager commands on Linux (they need `sudo`). Start the app with `./ytgrab --open`. The folder window uses `choose folder` on macOS and `zenity` or `kdialog` on Linux; without either, type the folder path.

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

- The first time, the page asks where to save downloads; nothing is downloaded until you choose. **Change…** next to **Save to** opens the folder window again, and **Type a path** lets you enter a full path instead. The folder applies to downloads that start afterwards and is remembered.
- The suggested folder is `Downloads\ytgrab` in your user folder; it is created when you pick it.
- The job history is stored in `%AppData%\ytgrab\jobs.db` on Windows (`~/.config/ytgrab` on Linux, `~/Library/Application Support/ytgrab` on macOS).

Optional environment variables, set before starting:

| Variable | Purpose |
| --- | --- |
| `YTGRAB_LISTEN_ADDR` | Loopback address and port, default `127.0.0.1:8787`. Only `127.0.0.1` or `::1` are accepted. |
| `YTGRAB_TOOLS_DIR` | Extra folder searched first for `yt-dlp`, `ffmpeg`, `ffprobe`, `deno`, `node`. |
| `YTGRAB_DATA_DIR` | Folder for the job database. |
| `YTGRAB_DOWNLOAD_DIR` | Initial output folder when none has been saved in the page. |

Command-line: `ytgrab --open` starts and opens the browser; `ytgrab doctor` and `ytgrab setup` are described under Install; `ytgrab --version` prints the version.

## Troubleshooting

| What you see | What to do |
| --- | --- |
| **Tools missing** in the header | Run `ytgrab.exe setup` in the YTGrab folder (or double-click **Start YTGrab.cmd** again), then reload the page. |
| The folder window doesn't appear | It may be behind other windows; check the taskbar. Without a folder window (some Linux desktops), use **Type a path**. |
| "YouTube is limiting requests from this network. Wait a while, then retry." | YouTube is rate-limiting or asking for a bot check. Wait (often 15–60 minutes) and press Retry. Avoid checking many links quickly. Signed-in downloads (cookies) are not supported in this version. |
| "yt-dlp could not download this video. Check that yt-dlp is up to date and retry." | YouTube changes often. Update yt-dlp: `ytgrab.exe setup --update-ytdlp` in the YTGrab folder, then Retry. |
| "This video is unavailable or private." | The video is private, removed, age- or region-restricted. Check the link in a browser. |
| "Couldn't load formats" | The same causes as above; the quick presets still work. |
| Video saved as `.mkv` or `.webm` | Best quality often uses VP9 or AV1 video, which isn't stored in MP4. Pick an H.264 row or **Up to 1080p** for `.mp4`. |
| Console shows `listen on 127.0.0.1:8787 … Only one usage of each socket address` | YTGrab is already running (open `http://127.0.0.1:8787/`), or another program uses the port. Set `YTGRAB_LISTEN_ADDR=127.0.0.1:8788` and start again. |
| The page says "The ytgrab server isn't responding" | The console window was closed. Start YTGrab again and reload the page. |
| Antivirus warns about `yt-dlp.exe` | It is a packaged Python program, which some scanners flag. Compare its SHA-256 with `manifest.json` and the official release. |
| "Choose an existing, writable absolute folder." | Enter a full path like `D:\Videos` for a folder that already exists. |
