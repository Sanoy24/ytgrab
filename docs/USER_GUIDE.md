# YTGrab user guide

YTGrab downloads YouTube videos and audio you are allowed to save, from a page in your browser. It runs only on your computer: the page is served from `127.0.0.1`, and files are written straight to your downloads folder by `yt-dlp`.

## Install on Windows

**With Scoop:** if you use [Scoop](https://scoop.sh), run:

```powershell
scoop bucket add sanoy24 https://github.com/Sanoy24/scoop-bucket
scoop install sanoy24/ytgrab
ytgrab --open
```

Scoop installs FFmpeg and Deno too and adds **YTGrab** to the Start menu; yt-dlp comes in the package. Upgrade with `scoop update ytgrab`, uninstall with `scoop uninstall ytgrab`. Your settings and history are kept either way.

**From the zip:**

1. Unzip `ytgrab-<version>-windows-amd64.zip` into a folder you keep, for example `C:\Apps\YTGrab`. Don't run it from inside the zip.
2. Double-click **Start YTGrab.cmd**. If Windows shows "Windows protected your PC", click **More info → Run anyway** (the app isn't code-signed). The first time, it runs a quick check. If something is missing, it runs setup, which asks before each step:
   - **yt-dlp**: downloaded from its official GitHub release and installed only if its checksum matches. A checked copy is already in the `tools` folder.
   - **FFmpeg** (merges video and audio, converts MP3) and **Deno** (runs YouTube's JavaScript checks): installed with `winget`.
3. Your browser opens `http://127.0.0.1:8787/`. Choose where downloads should go: **Choose folder…** opens the normal Windows folder window, or use the suggested `Downloads\ytgrab`.

YTGrab then keeps running without a window. Its icon sits in the notification area by the clock — on Windows 11, new icons start under the **^** arrow; drag it onto the taskbar to keep it in view. Click the icon to open the page; right-click it and choose **Quit YTGrab** to stop. Unfinished downloads continue from where they stopped next time. Point at the icon to see how many downloads are running and queued. When downloads finish or fail while no YTGrab page is open, Windows shows a notification; with the page open, use **Notify me** in the page instead, so you aren't told twice. Next time, **Start YTGrab.cmd** opens the app straight away.

The tray menu also has **Open downloads folder**, which opens the folder YTGrab currently saves to.

Started from a terminal (`ytgrab --open`), YTGrab stays in that terminal and prints its log there; it shows the tray icon too, and Ctrl+C stops it. When it runs without a window, its log is written to `ytgrab.log` in the data folder, and anything that stops it from starting is shown in a message box.

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

**With Homebrew:** if you use [Homebrew](https://brew.sh), run:

```sh
brew install sanoy24/tap/ytgrab
ytgrab --open
```

Homebrew installs yt-dlp, FFmpeg, and Deno with it, and nothing is quarantined, so there are no macOS warnings. Upgrade with `brew upgrade ytgrab`; when YouTube downloads start failing, `brew upgrade yt-dlp` (or the update button in the page) gets a newer yt-dlp. On Linux, add `zenity` from your package manager for the folder window.

**With the installer:** run this in Terminal:

```sh
curl -fsSL https://raw.githubusercontent.com/Sanoy24/ytgrab/main/install.sh | sh
```

The installer downloads the build for your system, checks it against the published SHA-256 checksum, installs it into `~/.local/share/ytgrab`, and adds a `ytgrab` command to `~/.local/bin`. It then offers to run `ytgrab setup`. If `~/.local/bin` isn't on your `PATH`, it prints the line to add to your shell's startup file. Start the app with `ytgrab --open`.

- **macOS:** the installer adds **YTGrab** to `~/Applications` (set `YTGRAB_NO_MENU=1` to skip that), so you can open it from Launchpad or Spotlight without a terminal. YTGrab then lives in the menu bar: its icon opens the page, opens your download folder, and quits, and macOS shows notifications when downloads finish. Setup installs FFmpeg and Deno with [Homebrew](https://brew.sh); without Homebrew it tells you what to install. Choose folder uses the standard macOS folder window.
- **Linux:** the installer adds **YTGrab** to your applications menu (set `YTGRAB_NO_MENU=1` to skip that). Started from the menu it has no terminal: on desktops with a tray (KDE, Ubuntu, and GNOME with the AppIndicator extension) its icon opens and quits it, as on Windows, and shows notifications when downloads finish. Without a tray, start it from a terminal instead. Setup installs yt-dlp and prints the commands for the rest, which need `sudo`:
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

Files include the video's title, channel, upload date, and chapters; M4A and MP3 files also get the thumbnail as cover art. The browser tab title shows progress, and **Notify me** (top right) sends a notification when a download finishes while YTGrab is in the background.

Files are named `Title [video-id] 1080p.mp4` (audio: `Title [video-id] 130k.m4a`), so different qualities of one video never overwrite each other.

## Download part of a video

For a single video, open **Only part of the video** below the formats and enter a **Start** and **End** time, like `1:05` and `2:30` (or `1:02:30` for longer videos). Leave Start empty to begin at the start, or End empty to go to the end. The clip is cut exactly at those times, which takes a little extra processing, and its file name includes the range (`Title [id] 720p 1m05s-2m30s.mp4`), so it never replaces a full download. A video can be in the queue once at a time, so add the full video and a clip of it one after the other.

## Download several videos at once

Paste several video links into the link field at once, one per line or separated by spaces (from a document, a chat, or a list you made). YTGrab shows them in a list like a playlist: untick any you don't want, choose a preset, and press **Add N videos**. Repeated links are added once. Playlist links and anything that isn't a single-video link are skipped, with a note saying how many; paste a playlist on its own to review its videos.

## Download a playlist

Paste a playlist link (`youtube.com/playlist?list=…`). YTGrab lists the first 50 videos, all ticked; **Load next 50** shows more. Up to 200 can be added at a time. Untick what you don't want, choose a preset (it applies to every video), and press **Add N videos**. Nothing is queued until you press that button. **Save in a folder named "…"** (on by default; YTGrab remembers if you turn it off) puts the playlist's videos in a folder of that name inside your download folder; characters that aren't allowed in folder names are left out. Private and deleted videos are skipped. For a link to one video inside a playlist, YTGrab downloads just that video and offers **Download the whole playlist instead**. YouTube Mixes (auto-generated radio lists) can't be downloaded as playlists.

## Queue and history

- **Cancel** stops a download and keeps its partial file. **Retry** resumes from that partial file.
- If YTGrab closes while downloading, the job appears as failed with "The app closed while this download was running"; **Retry** resumes it.
- Network hiccups are retried automatically up to three attempts.
- **Show in folder** opens your file manager with the finished file selected. **Copy path** copies its full path.
- **Remove** takes an entry off the list. For a finished download it asks first: **Remove from list** keeps the file, **Delete file too** deletes it.
- **Clear** (next to the History heading) removes all finished, failed, and cancelled entries at once. Your files are kept.

## Updating YTGrab

Once a day YTGrab checks GitHub for a newer release. When there is one, the page shows a notice with the command for the way you installed it: `scoop update ytgrab`, `brew upgrade ytgrab`, the curl installer, or `go install`. Quit YTGrab, run it, and start YTGrab again. For a copy unpacked from a zip, the notice links to the release page; download the new zip and replace the files in your YTGrab folder (your settings and history are kept, because they live in the data folder). **Not now** hides the notice until the next version.

## Settings and data

- The first time, the page asks where to save downloads; nothing is downloaded until you choose. **Change…** next to **Save to** opens the folder window again, and **Type a path** lets you enter a full path instead. The folder applies to downloads that start afterwards and is remembered.
- **More settings** (under **Save to**) holds **Parallel downloads** (1–4 at a time, default 2; more rarely helps because YouTube limits speed per network), **Default format** (the format new links start with), **Start with Windows**, and **YouTube sign-in**. Changes apply right away.
- **Speed limit (per download)** (in **More settings**) caps how fast each download runs: 0.5 to 10 MB/s, or no limit (the default). Use it to leave bandwidth for other things; with two downloads at 2 MB/s, YTGrab uses up to 4 MB/s. It applies to downloads that start after you change it.
- **Subtitles** (in **More settings**) is off by default. **Add to the video** puts subtitles inside the video file, where players offer them as a subtitle track; **Save as .srt file** saves them next to the video with the same name, which almost every player picks up. Choose the **Subtitle language**; YTGrab uses the creator's subtitles in that language, or YouTube's automatic ones when there are none, and videos with neither download without subtitles. It applies to video downloads, not audio. Fetching subtitles is one more request to YouTube, so if YouTube is limiting your network, the download waits with the others. Removing a download with its file doesn't remove a saved .srt.
- **Start with Windows** (on macOS **Open at login**, on Linux with a tray **Start when you log in**; in **More settings**, or from the tray or menu-bar menu) starts YTGrab in the tray when you sign in to Windows, without opening the browser; click the icon when you need it. It adds a `YTGrab` entry to your own startup apps (no administrator rights needed), which you can also see under Settings → Apps → Startup. If you move the YTGrab folder or Scoop updates it, the entry follows the next time YTGrab starts.
- **YouTube sign-in** (in **More settings**) is off by default. If YouTube keeps blocking downloads even after the automatic pauses, choose your browser there: yt-dlp then uses that browser's YouTube sign-in. YTGrab never reads or stores the sign-in itself. Heavy use can get an account flagged, so a secondary account is safer. On Windows, Firefox works best, because recent Chrome and Edge versions protect their sign-in data in a way yt-dlp can't read; close the browser if reading fails.
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
| "YouTube is limiting requests from this network" | YouTube is rate-limiting this connection or asking for a bot check. YTGrab pauses all downloads and format checks automatically (15 minutes, then 30 and 60 if it keeps happening), shows a countdown, and resumes on its own; queued downloads are kept. **Resume now** ends the pause early, for example after switching networks. A download blocked three times stops and can be retried later. Checking many links quickly makes blocks more likely. If blocks keep coming back, turn on **YouTube sign-in** (see Settings and data). |
| "YTGrab couldn't use your browser's YouTube sign-in" | Close that browser completely and retry, or choose another browser. On Windows, use Firefox. |
| "yt-dlp could not download this video. Check that yt-dlp is up to date and retry." | YouTube changes often. Click the tools pill at the top of the page and press **Update** next to yt-dlp (or run `ytgrab setup --update-ytdlp`), then Retry. The pill turns amber and says "yt-dlp update available" when a newer version exists. |
| "This video is unavailable or private." | The video is private, removed, age- or region-restricted. Check the link in a browser. |
| "Couldn't load formats" | The same causes as above; the quick presets still work. |
| Video saved as `.mkv` or `.webm` | Best quality often uses VP9 or AV1 video, which isn't stored in MP4. Pick an H.264 row or **Up to 1080p** for `.mp4`. |
| Starting YTGrab says "YTGrab is already running at …" | It is already open; the browser is brought to it (with `--open` or **Start YTGrab.cmd**). Only one copy runs at a time. |
| "listen on 127.0.0.1:8787 … Another program is using this port" | A different program uses the port. Set `YTGRAB_LISTEN_ADDR=127.0.0.1:8788` and start again. |
| "another YTGrab is using the data folder" | A second copy was started with a different port. Use the copy that is already running. |
| The page says "The ytgrab server isn't responding" | YTGrab was quit, or its terminal was closed. Start YTGrab again and reload the page. |
| Antivirus warns about `yt-dlp.exe` | It is a packaged Python program, which some scanners flag. Compare its SHA-256 with `manifest.json` and the official release. |
| "Only … free on the drive with your download folder" or "The drive … is full" | Free up space or choose a folder on another drive with **Change…**, then Retry. `ytgrab doctor` shows how much space is free. |
| "Choose an existing, writable absolute folder." | Enter a full path like `D:\Videos` for a folder that already exists. |
