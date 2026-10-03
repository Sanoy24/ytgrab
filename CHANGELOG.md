# Changelog

All notable changes to YTGrab. Versions follow [semantic versioning](https://semver.org/).

## [1.12.0] - 2026-10-03

YTGrab now downloads from X (Twitter) too.

### Added

- **X (Twitter) videos.** Paste a link to an X post with a video, such as `https://x.com/user/status/…`, and pick its quality or take its audio as M4A, MP3, Opus, FLAC, or WAV. Files are named after the post (`Text [post id] 1080x1920.mp4`), and the post's preview image shows in the Library. Portrait videos are labeled by their shorter side, so 1080x1920 is "1080p". X's limits never pause YouTube downloads.

- **Clear can delete files.** Clearing the Library now asks first, with an option to also delete the downloaded files. Files that can't be deleted (open in a player, say) keep their entry.

### Changed

- **YouTube sign-in** is now **Browser sign-in**, since it also lets yt-dlp open X posts that are shown only to signed-in users.

## [1.11.0] - 2026-10-03

### Security

- The page can no longer be shown inside another website's frame, which a site could otherwise use to trick you into clicking YTGrab's buttons. A Content-Security-Policy also limits the page to its own files and YouTube's thumbnails.

### Added

- **Even out loudness.** MP3, FLAC, and WAV downloads can be brought to a similar volume.
- **Search.** Type words instead of a link and press Enter to search YouTube, then pick a result.
- **Download window.** Let downloads start only between chosen hours, such as overnight; the queue says when waiting downloads will start.
- **Smarter Watching.** Filter a watch by minimum length (skip Shorts) or by words in the title, and choose to check every hour, 6 hours, or day. Upcoming premieres and live streams now wait until they can be downloaded instead of failing.
- **SponsorBlock.** Mark sponsor reads, self-promotion, and "like and subscribe" reminders as chapters, or cut them out, using the community SponsorBlock list.

## [1.10.0] - 2026-10-03

### Added

- **Watch channels and playlists.** In the new **Watching** view, add a channel or playlist once; YTGrab checks it every 6 hours and downloads new videos in the format you chose, into their own folder. Start with only new uploads or with the latest few, and check, pause, or stop any watch.
- **File names.** Choose to name files by title, channel and title, date and title, or to keep a folder per channel.
- **Split into chapters.** For videos with chapters, also save each chapter as its own numbered file, in a folder next to the full download.
- **Cover art for Opus and FLAC.** Like M4A and MP3, they now get the video's thumbnail as cover art.

### Fixed

- **Accessibility.** Small grey text now meets WCAG contrast in both themes, a "Skip to the link field" link helps keyboard users, navigation buttons read naturally to screen readers ("Library, 5 downloads"), and screen readers announce finished and failed downloads instead of every progress update.
- **Faster with a long history.** Unchanged downloads are no longer redrawn every second while something downloads.

## [1.9.0] - 2026-10-03

### Changed

- **A new look.** YTGrab has a sidebar with **Download**, **Library**, and **Settings**. Paste a link to see the video's thumbnail and choose a quality from a Video / Audio switch; downloads in progress show with thumbnails, and a small card in the sidebar follows them from any view. The Library can be searched. The page fits the window, so nothing important scrolls out of view.

### Added

- **More audio formats.** Opus (YouTube's own audio, usually without re-encoding), FLAC, and WAV, next to M4A and MP3.
- **Pause and resume.** Pause a download and resume it later from where it stopped; it keeps its place in the queue, even across restarts. Waiting downloads can be moved to the top of the queue.
- **Library actions.** Play a finished download in your default player, **Download again** in another format, and **Retry failed** to queue every failed or cancelled download at once.
- **Automatic yt-dlp updates.** YTGrab installs new yt-dlp releases by itself between downloads (on by default; turn it off in Settings), so YouTube changes break downloads less often.
- **Send to YTGrab.** A bookmark (drag it from Settings to your bookmarks bar) opens YTGrab from any YouTube page with that video ready to add.
- **Paste or drop anywhere.** Press Ctrl+V anywhere on the page, or drop a link onto it, to start a download.

## [1.8.0] - 2026-10-02

### Added

- **macOS menu bar.** YTGrab gets a menu-bar icon on macOS with the same menu, notifications, and an **Open at login** option. The installer adds **YTGrab** to `~/Applications`, so it opens from Launchpad or Spotlight without a terminal. Mac builds are now made on a Mac so they can include the icon.
- **Linux desktop.** The installer adds YTGrab to the applications menu, and on desktops with a tray (KDE, Ubuntu, GNOME with the AppIndicator extension) YTGrab gets the same tray icon, menu, notifications, and **Start when you log in** option as on Windows.
- **Part of a video.** Enter a start and end time to download just that section, cut exactly; the file name includes the range.
- **Playlist folders.** A playlist's videos can be saved into a folder named after the playlist (on by default, remembered if you turn it off).
- **Speed limit.** In More settings, cap each download at 0.5 to 10 MB/s to leave bandwidth for other things.
- **Subtitles.** In More settings, add subtitles in a chosen language to video downloads, either inside the video or as an .srt file next to it. Uses the creator's subtitles, or YouTube's automatic ones.
- **Several links at once.** Paste a list of video links (one per line or separated by spaces) to review them together and add them all with one format. Repeats are added once; playlist and other links are skipped with a note.
- **Update notices.** When a newer YTGrab is released, the page says so and shows the update command for how you installed it (Scoop, Homebrew, the installer, or `go install`), or links to the release for zip copies. It checks GitHub at most once a day.
- **Open downloads folder** in the tray menu.
- **Download status in the tray.** The tray icon's tooltip shows how many downloads are running and queued, and Windows shows a notification when downloads finish or fail while no YTGrab page is open.
- **Start with Windows.** Turn it on in More settings or from the tray menu, and YTGrab starts in the tray when you sign in, without opening the browser. The entry follows YTGrab if its folder moves or Scoop updates it.

### Fixed

- Opening More settings no longer shifts the whole page sideways, and YouTube sign-in now lines up with the other settings.

## [1.7.0] - 2026-10-02

### Added

- **Tray icon on Windows.** Started from the Start menu or **Start YTGrab.cmd**, YTGrab runs without a console window: click its icon by the clock to open the page, right-click it to quit. Its log goes to `ytgrab.log` in the data folder, and startup problems appear in a message box. Started from a terminal, it works as before and shows the icon too.

## [1.6.0] - 2026-10-02

### Added

- Install with Homebrew (`brew install sanoy24/tap/ytgrab`) on macOS and Linux, or with Scoop (`scoop install sanoy24/ytgrab`) on Windows. Both are updated automatically with each release.
- YTGrab has an icon: a download arrow on red. It shows on `ytgrab.exe`, in the Start menu for Scoop installs, and on the browser tab.

## [1.5.0] - 2026-10-02

### Added

- **More settings** under Save to: choose how many downloads run at once (1–4, applies immediately) and the format new links start with.
- Playlists of any length: the review list loads 50 videos at a time with **Load next 50**, and up to 200 can be added at once (previously 50).
- A download doesn't start when the drive has less than 256 MB free, and "disk full" errors are explained. `ytgrab doctor` shows free space for the download folder.

## [1.4.0] - 2026-10-02

### Added

- The browser tab title shows download progress, and an optional **Notify me** button (top right) sends a notification when a download finishes or fails while YTGrab is in the background.
- Downloads now include the title, artist, upload date, and chapters; M4A and MP3 files also get the thumbnail as cover art.

## [1.3.0] - 2026-10-01

### Added

- **Show in folder** for finished downloads opens the file manager with the file selected (Explorer, Finder, or the Linux file manager).
- **Remove** entries from the history; for finished downloads, choose whether to also delete the file. **Clear** removes all finished entries and keeps the files.
- Optional **YouTube sign-in** from your browser (off by default) for networks YouTube keeps blocking. yt-dlp reads the browser's sign-in directly; YTGrab never stores it. Cookie-reading problems get their own clear error.

## [1.2.0] - 2026-10-01

### Added

- **yt-dlp updates from the page.** YTGrab checks for a newer yt-dlp once a day; when one exists, the tools pill turns amber and an **Update** button installs it (checksum-verified, into YTGrab's own tools folder). `ytgrab doctor` shows available updates too.
- Linux ARM64 builds (`linux-arm64`), also supported by the installer.

### Fixed

- Starting YTGrab while it is already running now opens the running copy instead of failing with "port in use". Previously a second launch could mark the running copy's downloads as interrupted and start duplicate downloads before noticing; YTGrab now claims its port first and locks its data folder.

## [1.1.0] - 2026-10-01

### Added

- Automatic pause when YouTube limits the network: downloads and format checks wait (15, 30, then 60 minutes), a countdown shows when they resume, and blocked downloads retry on their own. **Resume now** ends the pause early.
- Downloads start a few seconds apart and yt-dlp spaces out its requests.
- One-line installer for macOS and Linux (`install.sh`), which avoids the macOS "Not Opened" warning.

### Fixed

- The program's `tools` folder is found when YTGrab is started through a symlink.

## [1.0.0] - 2026-09-26

First release: video and audio downloads with quick presets or exact formats, playlists of up to 50 videos with a review step, live progress, cancel/retry/resume, a native folder picker, `ytgrab setup` and `ytgrab doctor`, and builds for Windows, macOS, and Linux.

[1.12.0]: https://github.com/Sanoy24/ytgrab/compare/v1.11.0...v1.12.0
[1.11.0]: https://github.com/Sanoy24/ytgrab/compare/v1.10.0...v1.11.0
[1.10.0]: https://github.com/Sanoy24/ytgrab/compare/v1.9.0...v1.10.0
[1.9.0]: https://github.com/Sanoy24/ytgrab/compare/v1.8.0...v1.9.0
[1.8.0]: https://github.com/Sanoy24/ytgrab/compare/v1.7.0...v1.8.0
[1.7.0]: https://github.com/Sanoy24/ytgrab/compare/v1.6.0...v1.7.0
[1.6.0]: https://github.com/Sanoy24/ytgrab/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/Sanoy24/ytgrab/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/Sanoy24/ytgrab/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/Sanoy24/ytgrab/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/Sanoy24/ytgrab/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/Sanoy24/ytgrab/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/Sanoy24/ytgrab/releases/tag/v1.0.0
