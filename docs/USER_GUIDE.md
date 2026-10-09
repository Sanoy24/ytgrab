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

Homebrew installs yt-dlp, FFmpeg, and Deno with it, and nothing is quarantined, so there are no macOS warnings. Upgrade with `brew update && brew upgrade ytgrab` (`brew update` first, since Homebrew checks for new versions only once a day); when YouTube downloads start failing, `brew update && brew upgrade yt-dlp` (or the update button in the page) gets a newer yt-dlp. On Linux, add `zenity` from your package manager for the folder window.

**With the installer:** run this in Terminal:

```sh
curl -fsSL https://raw.githubusercontent.com/Sanoy24/ytgrab/main/install.sh | sh
```

The installer downloads the build for your system, checks it against the published SHA-256 checksum, installs it into `~/.local/share/ytgrab`, and adds a `ytgrab` command to `~/.local/bin`. It then offers to run `ytgrab setup`. If `~/.local/bin` isn't on your `PATH`, it prints the line to add to your shell's startup file. Start the app with `ytgrab --open`.

- **macOS:** the installer adds **YTGrab** to `~/Applications` (set `YTGRAB_NO_MENU=1` to skip that), so you can open it from Launchpad or Spotlight without a terminal. With Homebrew, run `ytgrab app` once to add it (`ytgrab app --remove` takes it out). YTGrab then lives in the menu bar: its icon opens the page, opens your download folder, and quits, and macOS shows notifications when downloads finish. Setup installs FFmpeg and Deno with [Homebrew](https://brew.sh); without Homebrew it tells you what to install. Choose folder uses the standard macOS folder window.
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
   - **M4A**: original audio, no re-encoding. **MP3**: converted, plays anywhere, uses more CPU. **Opus**: YouTube's own audio in an `.opus` file, usually without re-encoding. **FLAC**: lossless copy for editing software. **WAV**: uncompressed, for tools that need it; files are large.
3. When the check finishes, the presets are replaced by the real options: one row per resolution with its codec and estimated size, and each audio stream. H.264 picks are saved as `.mp4`. Use **Use standard presets instead** to switch back.
4. Press **Add to queue**. The queue shows live progress; merged downloads show the video part, then the audio part.

Files include the video's title, channel, upload date, and chapters; M4A, MP3, Opus, and FLAC files also get the thumbnail as cover art (WAV files have no place for it). The browser tab title shows progress, and **Notify me** (in the sidebar) sends a notification when a download finishes while YTGrab is in the background.

Files are named `Title [video-id] 1080p.mp4` (audio: `Title [video-id] 130k.m4a`), so different qualities of one video never overwrite each other.


**Saving data.** On a slow or metered connection, choose **Up to 480p** or **Up to 360p** for video, or **Small audio** for talks, lectures, and podcasts: it keeps YouTube's own low-bitrate Opus track, well under half the size of the M4A, with no re-encoding. In **Settings → Downloads → Download only between**, **Use night hours (22:00–08:00)** holds downloads for cheaper night data bundles.
## Watch channels and playlists

Open **Watching**, paste a channel link (`youtube.com/@name`, `/channel/…`, `/c/…`, or `/user/…`) or a playlist link, choose a format, and press **Start watching**. YTGrab then checks it (every 6 hours unless you choose otherwise) while it's running and adds new videos to the queue by themselves. Notifications and the tray tell you when they finish.

- **Start with** decides what happens right away: **Only new uploads** (the default) downloads nothing that's already there; the other choices also download the latest 1, 5, or 10 videos.
- Videos go into a folder named after the channel or playlist unless you untick that.
- **Filters and how often to check**: a **Minimum length** (2 minutes skips Shorts), **Only titles containing** (comma-separated words; a title needs one of them), and **Check every** hour, 6 hours, or day. Videos a filter skips are never downloaded later, even if you change the filter. Upcoming premieres and live streams are left until they can be downloaded.
- A channel check looks at its 30 newest uploads; a playlist check looks at the whole playlist (up to 500 videos). One check adds at most 25 videos; any more wait for the next check.
- **Check now** checks straight away, **Pause** stops checking until you resume, and **Stop watching** removes the watch but keeps what it downloaded.
- Checks use one request to YouTube each and go through the same pause as everything else when YouTube is limiting your network, so watching a few channels doesn't make blocks more likely. Watching hundreds might.
- **Vimeo**: watch a channel (`vimeo.com/channels/name`), a group (`vimeo.com/groups/name`), or a public showcase (`vimeo.com/showcase/123`); channels and groups check their newest videos like a YouTube channel, showcases their whole list. Vimeo no longer lets apps list a person's own videos (`vimeo.com/name`), so watch one of their channels or showcases instead. Vimeo doesn't list video lengths, so **Minimum length** lets every Vimeo video through.

### Podcast feeds

Each watch has a podcast feed of what it downloaded, so new episodes show up in a podcast app on your phone by themselves. Turn on phone access first (**Settings → Phone**; the phone doesn't need pairing for this), then press the feed button on the watch and copy the address into the app's **Add podcast by URL**.

- Use an app that refreshes feeds **on the phone itself**, such as **AntennaPod** or **Podcast Addict** on Android. Apps that refresh through their own servers, like Pocket Casts, Spotify, or Overcast, can't reach a computer on your home Wi-Fi.
- The app refreshes while the phone is on the same Wi-Fi as the computer and YTGrab is running; downloaded episodes then play anywhere.
- Episodes play best when the watch saves **M4A** or **MP3**. Video formats work in apps that play video.
- The feed lists the watch's 100 newest finished downloads whose files are still there.
- Each address carries a secret key. If one got out, **Settings → Phone → Make new feed addresses** retires all of them; copy the new ones into your apps. If the computer gets a new address on your Wi-Fi, the feed addresses change too.

## Split a video into chapters

When the video's creator marked chapters, the Download page says how many and offers **Also save each of the N chapters as its own file**. YTGrab then saves the whole video as usual, plus a folder of the same name with one file per chapter, numbered and titled (`01 Intro.m4a`, `02 Docs.m4a`, …). It works for video and audio formats; it can't be combined with **Only part of the video**. Deleting the download's file (from its menu or with **Clear**) also deletes its chapter files, and the folder unless you put other files in it.

## Download from X (Twitter)

Paste the link to an X post that has a video, such as `https://x.com/user/status/1234567890` (copy it with **Share → Copy link**; `twitter.com` links work too). YTGrab lists the post's video qualities with estimated sizes, and offers the audio as M4A or a conversion. X's files already contain sound, so nothing is merged; portrait videos are labeled by their shorter side (a 1080x1920 video is "1080p"). Files are named after the post's text and ID, such as `Text [1234567890] 720x1280.mp4`.

Most posts download without signing in. If X shows a post only to signed-in users (age-restricted or from a protected account you follow), turn on **Browser sign-in** in Settings with a browser where you're signed in to X. When a post has several videos, YTGrab lists them all ticked: untick any you don't want, choose a preset, and press **Add N videos**. Each is saved as its own file, with `-2`, `-3`, … after the post ID from the second video on. A link that ends in `/video/2` goes straight to that video's formats. Subtitles, chapters, and SponsorBlock are YouTube features and don't apply to X. If X limits requests, only that download fails: YouTube downloads carry on.

## Download from Reddit

Paste the link to a Reddit post with a video, such as `https://www.reddit.com/r/videos/comments/6rrwyj/…` (also `old.reddit.com`, `redd.it/…`, or a `v.redd.it/…` video link). YTGrab lists the video's qualities and audio with estimated sizes, like a YouTube video: pick one, or use a preset. Files are named after the post, such as `Title [6rrwyj] 720x1280.mp4`.

Reddit's **Share** button makes short `/s/…` links that only redirect, so YTGrab can't read them: open the post and copy the link from the address bar instead. Posts in age-restricted or private communities need **Browser sign-in** with a browser where you're signed in to Reddit. Subtitles, chapters, and SponsorBlock don't apply, and Reddit's limits never pause YouTube downloads.

## Download from Vimeo

Paste a Vimeo link: `https://vimeo.com/22439234`, an unlisted video's `vimeo.com/<id>/<key>` link, a channel or group link to a video, or a `player.vimeo.com` link. Vimeo's own pages ask downloaders to sign in, so YTGrab reads the video through Vimeo's embedded player, which plays public and unlisted videos for anyone. You get the video's qualities with estimated sizes and its audio, saved as MP4 or M4A and named like `The Mountain [22439234] 1280x720.mp4`.

Some videos can't be downloaded this way: when the owner lets the video play only on certain websites, YTGrab says so; private or password-protected videos need **Browser sign-in** with a browser where you're signed in to Vimeo. Copy-protected (DRM) videos can't be downloaded at all; YTGrab says so and removes the cover image it fetched.

## Download from Instagram

Paste the link to a public Instagram reel or post with a video, such as `https://www.instagram.com/reel/DeAVy0uTbPn/` (the link from **Share → Copy link** works as is). YTGrab lists the video's qualities and audio, like a YouTube video; Instagram doesn't say how long a video is, so no sizes are shown. Videos are saved as MP4, named after the post: `Video by account [DeAVy0uTbPn] 1080x1920.mp4`. When a post has several videos, YTGrab lists them all ticked, like an X post with several; their files get `.2`, `.3`, … after the code.

Public posts download without signing in. Posts from private accounts, age-restricted posts, and stories need **Browser sign-in** with a browser where you're signed in to Instagram; heavy use can get an account limited, so a secondary account is safer. Instagram's preview images expire after a while, so older entries in the Library may lose their picture; the files aren't affected.

## Search YouTube

Type words instead of a link in the link field, such as `big buck bunny`, and press Enter. YTGrab shows up to 12 videos with their thumbnail, channel, and length; click one to load its formats as if you had pasted its link. Searching only happens when you press Enter, and each search is one request to YouTube through the same pacing as everything else.

## Send to YTGrab from your browser

In **Settings**, drag the **Send to YTGrab** button to your browser's bookmarks bar (press Ctrl+Shift+B, or Cmd+Shift+B on a Mac, if the bar is hidden). Then, on any YouTube video or playlist page or an X post, click the bookmark: YTGrab opens, in the same tab each time, with that link in the field and its formats loading. Choose a format and press **Add to queue**. The bookmark only fills in the link; nothing is downloaded until you add it.

You can also press Ctrl+V anywhere on the YTGrab page, or drag a link from another window onto it.

## Browser extension

The **Download with YTGrab** extension adds a button to your browser's toolbar: on a video page, click it and YTGrab opens (or comes to the front) with that video's link ready to add. Right-click a link or a page for **Download with YTGrab** too. On X's timeline, the page is the whole feed, not one post: right-click the post's date (like "Oct 2"), which links to the post, and choose **Download link with YTGrab**, or open the post first. Nothing is downloaded until you press **Add to queue**, and the extension reads nothing from the pages you visit: it only passes the address you clicked to YTGrab on your own computer.

Install it from the release page: download `ytgrab-extension-<version>.zip` from [Releases](https://github.com/Sanoy24/ytgrab/releases) and extract it into a folder you keep.

- **Chrome, Edge, Brave:** open `chrome://extensions` (Edge: `edge://extensions`), turn on **Developer mode**, choose **Load unpacked**, and pick the extracted folder. Pin it from the puzzle-piece menu so the button stays visible.
- **Firefox:** open `about:debugging#/runtime/this-firefox`, choose **Load Temporary Add-on**, and pick `manifest.json` in the extracted folder. Firefox removes temporary add-ons when it closes, until the extension is published on Firefox's add-on site.

If YTGrab isn't running, the button shows a red **!**; point at it to see why. If you changed YTGrab's port (`YTGRAB_LISTEN_ADDR`), set the same port in the extension's options.

## Download part of a video

For a single video, open **Only part of the video** below the formats and enter a **Start** and **End** time, like `1:05` and `2:30` (or `1:02:30` for longer videos). Leave Start empty to begin at the start, or End empty to go to the end. The clip is cut exactly at those times, which takes a little extra processing, and its file name includes the range (`Title [id] 720p 1m05s-2m30s.mp4`), so it never replaces a full download. A video can be in the queue once at a time, so add the full video and a clip of it one after the other.

## Download several videos at once

Paste several video links (YouTube videos or X posts) into the link field at once, one per line or separated by spaces (from a document, a chat, or a list you made). YTGrab shows them in a list like a playlist: untick any you don't want, choose a preset, and press **Add N videos**. Repeated links are added once. Playlist links and anything that isn't a single-video link are skipped, with a note saying how many; paste a playlist on its own to review its videos.

## Download a playlist

Paste a playlist link (`youtube.com/playlist?list=…`). YTGrab lists the first 50 videos, all ticked; **Load next 50** shows more. Up to 200 can be added at a time. Untick what you don't want, choose a preset (it applies to every video), and press **Add N videos**. Nothing is queued until you press that button. **Save in a folder named "…"** (on by default; YTGrab remembers if you turn it off) puts the playlist's videos in a folder of that name inside your download folder; characters that aren't allowed in folder names are left out. Private and deleted videos are skipped. For a link to one video inside a playlist, YTGrab downloads just that video and offers **Download the whole playlist instead**. YouTube Mixes (auto-generated radio lists) can't be downloaded as playlists.

## Queue and history

- **Pause** stops a download but keeps it in the queue, marked Paused, with its partial file; **Resume** continues from where it stopped. Paused downloads stay paused when YTGrab restarts.
- The **↑** button on a waiting download moves it to the front, so it starts next.
- **Cancel** stops a download and moves it to the Library, keeping its partial file. **Retry** resumes from that partial file.
- If YTGrab closes while downloading, the job appears as failed with "The app closed while this download was running"; **Retry** resumes it.
- Network hiccups are retried automatically up to three attempts.
- The **Library** lists finished, failed, and cancelled downloads, with a search box and filters. For a finished download, the play button opens it in your default player, the folder button shows it in its folder, and **Download again** puts its link back on the Download page so you can choose another format. **Retry failed** queues every failed and cancelled download again at once, except videos that are deleted, private, or copy-protected, which a retry can't fix; **Remove unavailable** clears those from the list (each still has its own Retry button, in case a video comes back).
- **Show in folder** opens your file manager with the finished file selected. **Copy path** copies its full path.
- **Remove** takes an entry off the list. For a finished download it asks first: **Remove from list** keeps the file, **Delete file too** deletes it.
- **Sizes and sorting.** Each finished download shows its file's size, and the top of the Library adds them up ("12 files · 3.4 GB on disk"). A file you moved or deleted outside YTGrab is marked **file moved or deleted**. Sort by newest, oldest, largest, or title; the choice is remembered.
- **Export CSV** saves the list as shown (after search, filter, and sort) as a spreadsheet file with each download's title, site, link, format, state, size, date, and file.
- **Clear** (next to the Library filters) removes all finished, failed, and cancelled entries at once. It asks first: your files are kept unless you tick **Also delete the downloaded files**, which deletes each finished download's file too (only files YTGrab saved, recognized by the video ID in the name). A file that can't be deleted, for example because it's open in a player, stays in the list so you can try again.

## Updating YTGrab

Once a day YTGrab checks GitHub for a newer release. On Windows and Linux, a copy unpacked from a release archive or put in place by the install script can update itself: press **Update now** in the notice. YTGrab downloads the release for your system, checks it against the checksum published with it, makes sure the new version starts, replaces itself, and restarts in the background (with its tray icon); the page reloads when it's back. Your settings, history, and downloads are kept. It won't update while downloads are running: let them finish or cancel them first.

Otherwise the notice shows the command for the way you installed it: `scoop update ytgrab`, `brew update && brew upgrade ytgrab`, the curl installer, or `go install`. Quit YTGrab, run it, and start YTGrab again. For a copy unpacked from a zip, the notice links to the release page; download the new zip and replace the files in your YTGrab folder (your settings and history are kept, because they live in the data folder). **Not now** hides the notice until the next version.

## Settings and data

- The first time, the page asks where to save downloads; nothing is downloaded until you choose. **Change…** next to **Save to** opens the folder window again, and **Type a path** lets you enter a full path instead. The folder applies to downloads that start afterwards and is remembered.
- **More settings** (under **Save to**) holds **Parallel downloads** (1–4 at a time, default 2; more rarely helps because YouTube limits speed per network), **Default format** (the format new links start with), **Start with Windows**, and **Browser sign-in**. Changes apply right away.
- **File names** (in **Settings**): **Title** (the default, `Me at the zoo [jNQXAC9IVRw] 720p.mp4`), **Channel – Title** (`jawed - Me at the zoo [jNQXAC9IVRw] 720p.mp4`), **Date + Title** (upload date first, so files sort by date), **A folder per channel** (`jawed\Me at the zoo [jNQXAC9IVRw] 720p.mp4`), or **For Jellyfin and Plex** (see [Media servers](#media-servers)). Every style keeps the video ID in brackets: it keeps names unique and is how YTGrab knows a file is one it saved before deleting it. It applies to downloads that start after you change it.
- **Download only between** (in **Settings**, any time by default): choose hours, such as 01:00 and 07:00 (or 22:00 and 06:00 across midnight), and downloads start only then. Outside the window, new downloads wait in the queue and the Download page says when they'll start; downloads already running finish. Watches still check on schedule and their videos wait the same way.
- **Even out loudness** (in **Settings**, off by default) brings MP3, FLAC, and WAV downloads to a similar volume (about -16 LUFS, the level streaming services use), which helps music collections. M4A and Opus downloads are exact copies of YouTube's audio and stay untouched.
- **Sponsor segments** (in **Settings**, off by default) uses [SponsorBlock](https://sponsor.ajay.app/), a community-maintained list of sponsor reads, self-promotion, and "like and subscribe" reminders. **Mark as chapters** adds them as chapters you can skip in your player; **Cut them out** removes them from the file. Each download then also asks SponsorBlock's server about that video (only its ID is sent). Videos nobody has marked download unchanged.
- **Speed limit (per download)** (in **More settings**) caps how fast each download runs: 0.5 to 10 MB/s, or no limit (the default). Use it to leave bandwidth for other things; with two downloads at 2 MB/s, YTGrab uses up to 4 MB/s. It applies to downloads that start after you change it. Parts of videos (see below) are cut by FFmpeg, which doesn't follow the limit.
- **Subtitles** (in **More settings**) is off by default. **Add to the video** puts subtitles inside the video file, where players offer them as a subtitle track; **Save as .srt file** saves them next to the video with the same name, which almost every player picks up. Choose the **Subtitle language**; YTGrab uses the creator's subtitles in that language, or YouTube's automatic ones when there are none, and videos with neither download without subtitles. It applies to video downloads, not audio. Fetching subtitles is one more request to YouTube, so if YouTube is limiting your network, the download waits with the others. Removing a download with its file doesn't remove a saved .srt.
- **Keep yt-dlp up to date** (in **Settings**, on by default): when a newer yt-dlp is released, YTGrab installs it by itself between downloads, from yt-dlp's official release and only if its checksum matches. It checks a few minutes after starting and then every half hour. Turn it off to update only from the tools status instead.
- **Start with Windows** (on macOS **Open at login**, on Linux with a tray **Start when you log in**; in **More settings**, or from the tray or menu-bar menu) starts YTGrab in the tray when you sign in to Windows, without opening the browser; click the icon when you need it. It adds a `YTGrab` entry to your own startup apps (no administrator rights needed), which you can also see under Settings → Apps → Startup. If you move the YTGrab folder or Scoop updates it, the entry follows the next time YTGrab starts.
- **Browser sign-in** (in **More settings**) is off by default. If YouTube keeps blocking downloads even after the automatic pauses, or an X post is shown only to signed-in users, choose your browser there: yt-dlp then uses that browser's sign-in to those sites. YTGrab never reads or stores the sign-in itself. Heavy use can get an account flagged, so a secondary account is safer. On Windows, Firefox works best, because recent Chrome and Edge versions protect their sign-in data in a way yt-dlp can't read; close the browser if reading fails.
  On Windows, Chrome, Edge, and other Chromium browsers usually lock their sign-in data so yt-dlp can't read it; YTGrab then downloads without signing in. Instead, export a cookies.txt file from your browser (with a "cookies.txt" export extension, in Netscape format; yt-dlp's guide recommends exporting from a private window you then close, so YouTube doesn't replace those cookies) and choose **Use a cookies.txt file…**. YTGrab checks the file, keeps it only in its data folder, and deletes it when you turn sign-in off or pick a browser. Treat the file like a password.
- The suggested folder is `Downloads\ytgrab` in your user folder; it is created when you pick it.
- The job history is stored in `%AppData%\ytgrab\jobs.db` on Windows (`~/.config/ytgrab` on Linux, `~/Library/Application Support/ytgrab` on macOS).

Optional environment variables, set before starting:

| Variable | Purpose |
| --- | --- |
| `YTGRAB_LISTEN_ADDR` | Loopback address and port, default `127.0.0.1:8787`. Only `127.0.0.1` or `::1` are accepted. |
| `YTGRAB_TOOLS_DIR` | Extra folder searched first for `yt-dlp`, `ffmpeg`, `ffprobe`, `deno`, `node`. |
| `YTGRAB_DATA_DIR` | Folder for the job database. |
| `YTGRAB_DOWNLOAD_DIR` | Initial output folder when none has been saved in the page. |
| `YTGRAB_NO_TRAY` | Set to `1` to run without the tray or menu-bar icon. |

Command-line: `ytgrab --open` starts and opens the browser; `ytgrab doctor` and `ytgrab setup` are described under Install; `ytgrab --version` prints the version.

### yt-dlp builds

YTGrab keeps yt-dlp, the tool that does the downloading, up to date with its **Stable** releases. When YouTube changes something and downloads start failing, the fix usually reaches yt-dlp's **Nightly** builds a day or two earlier: choose Nightly in **Settings → This computer → yt-dlp builds**, and YTGrab installs it between downloads (checked against its published checksum). Switch back to Stable once the fix is in a release; YTGrab then installs the latest stable build again.

### About YTGrab

The last card in **Settings** shows your YTGrab version and whether it's up to date, plus the yt-dlp, FFmpeg, and JavaScript runtime versions it uses. **Check for updates** looks for a new release right away. The version also shows at the bottom of the sidebar; click it to jump here.

### Back up and restore

In **Settings → Back up and restore**, **Save a backup** saves a `ytgrab-backup-<date>.json` file with your settings, your watched channels and playlists (with the videos each has already seen, so a restored watch doesn't download them again), and your Library (up to the 500 most recent entries; the queue isn't included). **Restore from a file…** adds a backup to this copy: watches and Library entries already here are kept, and settings are applied through their usual checks. A setting that doesn't fit this computer, like a download folder that doesn't exist here, keeps its current value, and the page says which. **Start with Windows** belongs to each computer and isn't saved.
For other tools, **Save a yt-dlp archive** saves your finished YouTube and Vimeo downloads in yt-dlp's `--download-archive` format (one `youtube <id>` line each), so a yt-dlp script given that file skips videos you already have. X, Reddit, and Instagram downloads aren't included: YTGrab knows them by the post's ID, not the one yt-dlp records.

### Metadata files

**Save metadata files** (Settings → Downloads) keeps three files beside each new download, named like it: `.info.json` (everything yt-dlp knows about the video), `.description`, and the thumbnail as `.jpg`. Archives and media servers such as Jellyfin read them. Deleting the download with its file deletes them too, along with any subtitle files.

### Media servers

The **For Jellyfin and Plex** file-name style (Settings → Downloads → File names) saves videos the way media servers expect a TV show: the channel is the show, the upload year is the season, and the upload month and day are the episode number.

```
jawed\
  tvshow.nfo
  Season 2005\
    jawed - S2005E0424 - Me at the zoo [jNQXAC9IVRw] 720p.mp4
    jawed - S2005E0424 - Me at the zoo [jNQXAC9IVRw] 720p.nfo
    jawed - S2005E0424 - Me at the zoo [jNQXAC9IVRw] 720p-thumb.jpg
```

Add the download folder to Jellyfin, Kodi, or Emby as a **Shows** library and they read the title, date, description, and poster from these files. Plex reads the folders and names; for the descriptions, add an NFO agent such as XBMCnfoTVImporter. YTGrab writes `tvshow.nfo` only when the channel has none, so your own edits to it are kept. Deleting a download with its file deletes its `.nfo` and poster too, and the show's folder once its last episode is gone. Audio downloads get the same folders and names, without the `.nfo` and poster.

### Phone

**Settings → Phone → Use YTGrab from your phone** (off by default) lets a phone on the same Wi-Fi use YTGrab running on your computer: paste links, follow the queue, manage Watching, and save finished downloads to the phone. The computer still does the downloading.

1. Turn it on. The first time, Windows asks whether YTGrab may use the network: allow it on **private networks**.
2. Click **Show a code** and scan it with the phone's camera. The phone opens YTGrab; the code works once, for 10 minutes.
3. Next time, open the address shown under the code (like `http://192.168.1.5:8788`) on the phone. Bookmark it or add it to the home screen.

On the phone, a finished download has **Play**, which plays it in the browser over Wi-Fi, and **Save to phone**, which copies it to the phone's Downloads. Chrome warns that the file "can't be downloaded securely" because the connection isn't encrypted; tap **Keep**, since the file comes straight from your computer. The phone can't change settings, delete files on the computer, or reach anything else on it. **Paired phones** lists each phone with a **Remove** button; turning phone access off removes them all. The connection isn't encrypted, so use it on your home Wi-Fi or another network you trust, not public Wi-Fi.

Your router can give the computer a new address, for example after it restarts. The phone then can't reach YTGrab: the computer shows **Your phone can't reach YTGrab right now** with **Show the code**, and scanning it reconnects the phone (it replaces the old entry). To keep the address from changing, reserve it for this computer in your router's settings, usually called *DHCP reservation* or *static lease*.

If the phone can't connect, check that both are on the same Wi-Fi (guest networks often keep devices apart), and that Windows Firewall allows YTGrab on private networks. A VPN on the computer can also hide it from the phone.

## Troubleshooting

A failed download explains what went wrong in plain words. For anything you can't fix from that, press its **Copy report** button (or **Copy diagnostic report** in Settings → About) and paste the result into a [bug report](https://github.com/Sanoy24/ytgrab/issues/new): it lists your YTGrab, yt-dlp, FFmpeg, and JavaScript runtime versions, the link, and yt-dlp's own error lines. It contains the video's link, so check it before sharing.

| What you see | What to do |
| --- | --- |
| "Windows protected your PC" when starting | The app isn't code-signed. Click **More info → Run anyway**. |
| macOS says "ytgrab" Not Opened, or Apple could not verify it | Browser downloads are quarantined and the app isn't signed. Install with the `curl` installer instead, or run `xattr -c ytgrab` in its folder, or click **Open Anyway** in **System Settings → Privacy & Security**. |
| **Tools missing** in the header | Run `ytgrab.exe setup` in the YTGrab folder (or double-click **Start YTGrab.cmd** again), then reload the page. |
| The folder window doesn't appear | It may be behind other windows; check the taskbar. Without a folder window (some Linux desktops), use **Type a path**. |
| "YouTube is limiting requests from this network" | YouTube is rate-limiting this connection or asking for a bot check. YTGrab pauses all downloads and format checks automatically (15 minutes, then 30 and 60 if it keeps happening), shows a countdown, and resumes on its own; queued downloads are kept. **Resume now** ends the pause early, for example after switching networks. A download blocked three times stops and can be retried later. Checking many links quickly makes blocks more likely. If blocks keep coming back, turn on **Browser sign-in** (see Settings and data). |
| "YouTube wants to check that this network isn't a bot" | YouTube sends your whole internet address to its "are you a robot" page. On mobile and some home internet many people share one address, so this can happen even if you haven't downloaded anything. YTGrab works around it by itself: it switches to YouTube's embedded players (the ones on other websites and TVs), which still get every quality, for the next six hours. You see this message only when that doesn't work either, usually for a video whose owner turned off embedding. Then turn on **Browser sign-in**, try another network such as a phone hotspot, or wait a few hours. |
| "X is limiting requests from this network" | X is rate-limiting this connection. Wait a few minutes and retry; YouTube downloads aren't affected. |
| "X shows this post only to signed-in users" | Turn on **Browser sign-in** with a browser where you're signed in to X, then retry. |
| "YTGrab couldn't use your browser's sign-in" | Close that browser completely and retry, or choose another browser. On Windows, use Firefox. |
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
