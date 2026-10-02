# Changelog

All notable changes to YTGrab. Versions follow [semantic versioning](https://semver.org/).

## [Unreleased]

### Added

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

[Unreleased]: https://github.com/Sanoy24/ytgrab/compare/v1.7.0...HEAD
[1.7.0]: https://github.com/Sanoy24/ytgrab/compare/v1.6.0...v1.7.0
[1.6.0]: https://github.com/Sanoy24/ytgrab/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/Sanoy24/ytgrab/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/Sanoy24/ytgrab/compare/v1.3.0...v1.4.0
[1.3.0]: https://github.com/Sanoy24/ytgrab/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/Sanoy24/ytgrab/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/Sanoy24/ytgrab/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/Sanoy24/ytgrab/releases/tag/v1.0.0
