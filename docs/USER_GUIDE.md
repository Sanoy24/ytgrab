# YTGrab user guide

YTGrab downloads YouTube videos and audio you are allowed to save, from a page in your browser. It runs only on your computer: the page is served from `127.0.0.1`, and files are written straight to your downloads folder by `yt-dlp`.

## Install (Windows release)

1. Unzip `ytgrab-<version>-windows-amd64.zip` into a folder you keep, for example `C:\Apps\YTGrab`. Don't run it from inside the zip.
2. Double-click **Start YTGrab.cmd**. If Windows shows "Windows protected your PC", click **More info → Run anyway** (the app isn't code-signed). The first time, it runs a quick check. If something is missing, it runs setup, which asks before each step:
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

### macOS and Linux

Run this in Terminal:

```sh
curl -fsSL https://raw.githubusercontent.com/Sanoy24/ytgrab/main/install.sh | sh
```

The installer downloads the build for your system, checks it against the published SHA-256 checksum, installs it into `~/.local/share/ytgrab`, and adds a `ytgrab` command to `~/.local/bin`. It then offers to run `ytgrab setup`. If `~/.local/bin` isn't on your `PATH`, it prints the line to add to your shell's startup file. Start the app with `ytgrab --open`.

- **macOS:** setup installs FFmpeg and Deno with [Homebrew](https://brew.sh); without Homebrew it tells you what to install. Choose folder uses the standard macOS folder window.
- **Linux:** setup installs yt-dlp and prints the commands for the rest, which need `sudo`:
  ```sh
  sudo apt install ffmpeg zenity unzip   # Debian/Ubuntu
  curl -fsSL https://deno.land/install.sh | sh
  ```
  On Fedora use `sudo dnf install ffmpeg zenity unzip` (FFmpeg comes from RPM Fusion); on Arch, `sudo pacman -S ffmpeg zenity unzip`. Open a new terminal after installing Deno. `zenity` (or `kdialog` on KDE) provides the folder window; without it, type the folder path.

Run the installer again to upgrade; yt-dlp and your settings are kept. Installer options (set before `sh`): `YTGRAB_VERSION=1.0.0` for a specific version, `YTGRAB_INSTALL_DIR` and `YTGRAB_BIN_DIR` for other locations, `YTGRAB_NO_SETUP=1` to skip the setup question. To uninstall, delete `~/.local/share/ytgrab` and `~/.local/bin/ytgrab`.

**Manual install:** download `ytgrab-<version>-darwin-arm64.tar.gz` (Apple silicon), `-darwin-amd64` (Intel Mac), `-linux-amd64`, or `-linux-arm64` from the releases page, then:

```sh
mkdir -p ~/.local/share/ytgrab
tar -xzf ~/Downloads/ytgrab-*.tar.gz -C ~/.local/share/ytgrab
cd ~/.local/share/ytgrab
chmod +x ytgrab
xattr -c ytgrab 2>/dev/null   # macOS: browser downloads are quarantined and the app isn't signed
./ytgrab setup
./ytgrab --open
```

Other processor types can install with Go: `go install github.com/Sanoy24/ytgrab/cmd/ytgrab@latest`.

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
| "Windows protected your PC" when starting | The app isn't code-signed. Click **More info → Run anyway**. |
| macOS says "ytgrab" Not Opened, or Apple could not verify it | Browser downloads are quarantined and the app isn't signed. Install with the `curl` installer instead, or run `xattr -c ytgrab` in its folder, or click **Open Anyway** in **System Settings → Privacy & Security**. |
| **Tools missing** in the header | Run `ytgrab.exe setup` in the YTGrab folder (or double-click **Start YTGrab.cmd** again), then reload the page. |
| The folder window doesn't appear | It may be behind other windows; check the taskbar. Without a folder window (some Linux desktops), use **Type a path**. |
| "YouTube is limiting requests from this network" | YouTube is rate-limiting this connection or asking for a bot check. YTGrab pauses all downloads and format checks automatically (15 minutes, then 30 and 60 if it keeps happening), shows a countdown, and resumes on its own; queued downloads are kept. **Resume now** ends the pause early, for example after switching networks. A download blocked three times stops and can be retried later. Checking many links quickly makes blocks more likely. Signed-in downloads (cookies) are not supported in this version. |
| "yt-dlp could not download this video. Check that yt-dlp is up to date and retry." | YouTube changes often. Click the tools pill at the top of the page and press **Update** next to yt-dlp (or run `ytgrab setup --update-ytdlp`), then Retry. The pill turns amber and says "yt-dlp update available" when a newer version exists. |
| "This video is unavailable or private." | The video is private, removed, age- or region-restricted. Check the link in a browser. |
| "Couldn't load formats" | The same causes as above; the quick presets still work. |
| Video saved as `.mkv` or `.webm` | Best quality often uses VP9 or AV1 video, which isn't stored in MP4. Pick an H.264 row or **Up to 1080p** for `.mp4`. |
| Starting YTGrab says "YTGrab is already running at …" | It is already open; the browser is brought to it (with `--open` or **Start YTGrab.cmd**). Only one copy runs at a time. |
| "listen on 127.0.0.1:8787 … Another program is using this port" | A different program uses the port. Set `YTGRAB_LISTEN_ADDR=127.0.0.1:8788` and start again. |
| "another YTGrab is using the data folder" | A second copy was started with a different port. Use the copy that is already running. |
| The page says "The ytgrab server isn't responding" | The console window was closed. Start YTGrab again and reload the page. |
| Antivirus warns about `yt-dlp.exe` | It is a packaged Python program, which some scanners flag. Compare its SHA-256 with `manifest.json` and the official release. |
| "Choose an existing, writable absolute folder." | Enter a full path like `D:\Videos` for a folder that already exists. |
