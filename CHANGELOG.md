# Changelog

All notable changes to YTGrab. Versions follow [semantic versioning](https://semver.org/).

## [Unreleased]

## [1.18.0] - 2026-10-05

YTGrab as an archiver: media-server folders, metadata files, and downloads from your phone.

### Added

- **Save metadata files.** Settings → Downloads can keep each video's details (yt-dlp's info.json), description, and thumbnail as files beside it, for archives and media servers.
- **Save a yt-dlp archive.** Settings → Backup saves your finished YouTube and Vimeo downloads as a yt-dlp `--download-archive` file, so yt-dlp scripts and other tools skip what you already have.
- **Jellyfin and Plex layout.** A new file-name style, **For Jellyfin and Plex**, saves each channel as a show: a `Season <year>` folder per upload year, episodes named `S2005E0424` by upload date, a poster, and `.nfo` files with the title, date, and description, so Jellyfin, Kodi, Emby, and Plex list videos with their details.
- **Use YTGrab from your phone.** Settings → Phone (off by default) lets phones on the same Wi-Fi add links, follow downloads, use Watching, and **Save to phone** finished downloads. Pair a phone by scanning a QR code; only paired phones get in, they can't change settings or delete files on the computer, and you can remove one any time. If the router gives the computer a new address, YTGrab says so and shows a new code to reconnect the phone.

### Fixed

- Deleting a download's file now also deletes the subtitle files and other files yt-dlp saved beside it.
- **Downloads keep working through YouTube's bot check.** YouTube sometimes asks a whole internet address to confirm it isn't a bot, which on shared mobile and home internet happens without any downloading, and every YouTube download and format check then failed. YTGrab now switches to YouTube's embedded players, which still get every quality, for six hours, and goes back once the check is over. If a video won't play embedded either, the message says what helps (Browser sign-in, another network) instead of only asking you to wait.

## [1.17.0] - 2026-10-05

Steadier YouTube downloads, clearer errors, and data-saving formats.

### Added

- **Data-saver formats.** New presets for slow or metered connections: video up to 480p or 360p, and **Small audio**, YouTube's own low-bitrate Opus kept as is (a 10 MB M4A becomes about 4 MB), plenty for talks and lectures. The download window gets a one-click **Use night hours (22:00–08:00)** for cheaper night data bundles.
- **Nightly yt-dlp builds.** Settings → This computer → **yt-dlp builds** switches to Nightly, where fixes for YouTube changes arrive a day or two before a stable release, and back. YTGrab installs the chosen build (with its checksum) between downloads, and errors from YouTube changes now suggest it.
- **Sign in with a cookies.txt file.** Browser sign-in can use a cookies.txt exported from your browser, which works when a browser's own sign-in can't be read (Chrome and Edge on Windows, usually). YTGrab checks the file, keeps it only in its data folder, and deletes it when you switch sign-in off or to a browser.
- **Copy report.** A failed download has a **Copy report** button, and Settings → About has **Copy diagnostic report**: versions, settings, the error, and yt-dlp's own words, ready to paste into a bug report.

### Changed

- When a browser's sign-in can't be read, downloads and format checks try again without signing in instead of failing, and choosing Chrome, Edge, or another Chromium browser on Windows explains the problem up front.
- Clearer messages for common YouTube failures that used to say only "could not download": age-restricted and members-only videos, premieres that haven't started, a quality no longer offered, HTTP 403 refusals, and YouTube changes that need a newer yt-dlp.

### Fixed

- yt-dlp now uses exactly the JavaScript runtime YTGrab checked. Before, a Deno in YTGrab's tools folder was missed when it wasn't also on PATH, and an outdated Deno could be used even when a current Node was installed, both causing YouTube failures while YTGrab said "Tools ready".

## [1.16.1] - 2026-10-04

### Fixed

- Scrolling past the end of Settings could slide the whole window up, leaving a black band at the bottom. A hidden file picker stretched the page; it now stays in its row.

## [1.16.0] - 2026-10-04

### Changed

- **A clearer Settings page.** Settings are grouped into Downloads, Audio & video, Browser, This computer, Backup, and About, with a menu to jump between them. Each setting is one row with a short explanation beside its control, in larger, easier-to-read text.

## [1.15.0] - 2026-10-04

### Added

- **About YTGrab.** Settings shows which YTGrab, yt-dlp, FFmpeg, and JavaScript runtime you have, with **Check for updates** and links to what's new, the user guide, and reporting a problem. The sidebar shows the version too; click it to open About.

### Fixed

- Sending X's timeline (`x.com/home`) with the extension opened YTGrab with a link it can't download and no explanation. The extension now says to right-click the post's date instead, and YTGrab explains a handed-over link it can't use straight away.

## [1.14.0] - 2026-10-04

A browser extension, Vimeo watching, and backups.

### Added

- **Back up and restore.** Save your settings, watched channels, and Library to a file, and restore it after reinstalling or on another computer. Restored watches remember what they've already downloaded.
- **Browser extension.** Download with YTGrab (Chrome, Edge, Firefox) adds a toolbar button and a right-click item that send the video you're watching to YTGrab, reusing its tab. It reads nothing from your pages and needs no account. Get it from the release page.
- **Watch Vimeo.** Follow a Vimeo channel, group, or public showcase, and new videos download by themselves like a watched YouTube channel.

### Changed

- **Retry failed** skips downloads that can never work (deleted, private, or copy-protected videos), and a new **Remove unavailable** button clears them from the Library in one click.
- New YTGrab versions show up sooner: YTGrab looks every 6 hours instead of once a day, and again when you open the page if its last look is over an hour old.

## [1.13.1] - 2026-10-03

### Fixed

- Playlists with deleted or private videos that YouTube lists without a title queued those videos, which then failed. They are now skipped with the other unavailable videos, and Watching skips them too.

## [1.13.0] - 2026-10-03

YTGrab now downloads from Vimeo, Instagram, and Reddit too, and can update itself.

### Added

- **Update with one click.** On Windows and Linux, a copy unpacked from a release archive or installed with the install script can update itself from the update notice: YTGrab downloads the release, checks its checksum and that it starts, replaces itself, and restarts.
- **Vimeo videos.** Paste a Vimeo link, including unlisted ones, and pick its quality or audio. YTGrab reads videos through Vimeo's embedded player, which doesn't need signing in; videos limited to certain websites, private ones, and copy-protected ones say why they can't be downloaded.
- **Instagram videos.** Paste a link to a public Instagram reel or post and pick its quality or audio, or use a preset; posts with several videos list them all. Private and age-restricted posts work with Browser sign-in.
- **Reddit videos.** Paste a link to a Reddit post with a video (`reddit.com`, `old.reddit.com`, `redd.it`, or `v.redd.it`) and pick its quality or audio, or use a preset. Reddit's limits never pause YouTube downloads.
- **Library sizes, sorting, and export.** Finished downloads show their file size, the Library totals them, and files moved or deleted outside YTGrab are marked. Sort by newest, oldest, largest, or title, and export the list as a CSV file.
- **X posts with several videos.** YTGrab lists every video of the post, all ticked; untick any and add the rest with one preset. Each is saved as its own file. A `/video/2` link goes straight to that video.

### Changed

- Deleting a download's file also deletes its chapter files from **Split into chapters**, and their folder when nothing else is in it.
- The **Send to YTGrab** bookmark works on Vimeo, X, Reddit, and Instagram pages too.

### Fixed

- Several X links pasted at once were skipped as if they weren't video links.
- Inspecting an X post with several videos failed, and downloading one could fetch every video into a single file name.

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

[1.18.0]: https://github.com/Sanoy24/ytgrab/compare/v1.17.0...v1.18.0
[1.17.0]: https://github.com/Sanoy24/ytgrab/compare/v1.16.1...v1.17.0
[1.16.1]: https://github.com/Sanoy24/ytgrab/compare/v1.16.0...v1.16.1
[1.16.0]: https://github.com/Sanoy24/ytgrab/compare/v1.15.0...v1.16.0
[1.15.0]: https://github.com/Sanoy24/ytgrab/compare/v1.14.0...v1.15.0
[1.14.0]: https://github.com/Sanoy24/ytgrab/compare/v1.13.1...v1.14.0
[1.13.1]: https://github.com/Sanoy24/ytgrab/compare/v1.13.0...v1.13.1
[1.13.0]: https://github.com/Sanoy24/ytgrab/compare/v1.12.0...v1.13.0
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
