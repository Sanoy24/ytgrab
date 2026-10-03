# Development

## Requirements

- [Go](https://go.dev/dl/) 1.25 or newer
- yt-dlp, FFmpeg/FFprobe, and Deno or Node for real downloads (`go run ./cmd/ytgrab setup` installs them)
- Node, optionally, for checking the browser scripts

## Run from source

```sh
go run ./cmd/ytgrab setup   # first time: install missing tools
go run ./cmd/ytgrab --open
```

`setup` puts yt-dlp in `tools/` at the repository root (git-ignored). The UI in `web/static` is embedded at build time, so restart after editing it.

Configuration comes from environment variables:

| Variable | Default |
| --- | --- |
| `YTGRAB_LISTEN_ADDR` | `127.0.0.1:8787` (loopback addresses only) |
| `YTGRAB_TOOLS_DIR` | searched before the other tool locations |
| `YTGRAB_DATA_DIR` | `ytgrab` in the user config folder (`%AppData%` on Windows) |
| `YTGRAB_DOWNLOAD_DIR` | `Downloads/ytgrab`; setting it skips the first-run folder choice |
| `YTGRAB_NO_TRAY` | `1` turns off the tray or menu-bar icon (for example on a machine without a desktop) |

Use a separate `YTGRAB_DATA_DIR` while developing to keep your own settings and history untouched.

## Tests

```sh
go vet ./...
go test ./...
```

Tests that need the outside world are opt-in:

```sh
# A real download (needs tools/yt-dlp and internet access)
YTGRAB_INTEGRATION=1 go test ./internal/app -run TestIntegrationDownload -v

# Opens the real folder window on your desktop
YTGRAB_MANUAL_PICKER=1 go test ./internal/picker -run TestManualPick -v
```

The browser UI has its own tests in `web/` (Node 22 or newer):

```sh
cd web
npm ci
npm test                          # unit tests for link checks and format grouping
npx playwright install chromium   # once
npx playwright test               # drives the page in Chromium using its sample data
```

The end-to-end tests use the page's `?fixture=` modes, so they need no server, tools, or network. CI runs both on every push.

`e2e/a11y.spec.js` runs [axe-core](https://github.com/dequelabs/axe-core) against every view in light and dark mode and fails on any WCAG 2.1 A or AA violation; it also checks the skip link and the navigation buttons' spoken names. The `?fixture=big-library` mode adds 400 finished downloads, and a test checks that the Library isn't redrawn on every progress update.

## Previewing the UI without the server

Serve `web/static` with any static server and add a `fixture` parameter to use sample data:

```sh
python -m http.server 8765 --bind 127.0.0.1 --directory web/static
```

| URL | Shows |
| --- | --- |
| `/?fixture=default` | Active, queued, finished, failed, and cancelled jobs with simulated progress |
| `/?fixture=empty` | Empty queue and history |
| `/?fixture=first-run` | The first-run folder choice (the first pick is cancelled) |
| `/?fixture=no-picker` | No folder window available; typed path only |
| `/?fixture=degraded` | Missing tools |
| `/?fixture=error` | Server unreachable |
| `/?fixture=loading` | Loading states that never finish |
| `/?fixture=big-library` | The default jobs plus 400 finished downloads, for checking speed |
| `/?fixture=blocked` | YouTube rate limiting during format checks |
| `/?fixture=expired` | An expired inspection that is re-checked automatically |
| `/?fixture=outdated` | A yt-dlp update available, with the Update button |
| `/?fixture=cooldown` | The YouTube pause banner with its countdown and Resume now |

Without `fixture`, the page calls the real API, which a static server doesn't have.

## Releasing

Add a section for the new version at the top of `CHANGELOG.md`, commit, and push a tag:

```sh
git tag v1.3.0
git push origin v1.3.0
```

The [release workflow](../.github/workflows/release.yml) downloads the official `yt-dlp.exe` and its checksums, runs the tests, builds Windows, Linux (x86 and ARM), and macOS archives, and publishes a GitHub release whose notes come from the changelog. A tag with a suffix, such as `v1.3.0-rc.1`, becomes a pre-release, which the installer and "latest" links ignore.

For stable releases, a second job renders the Homebrew formula and Scoop manifest from the templates in [`packaging/`](../packaging/) with [`scripts/render-packages.sh`](../scripts/render-packages.sh) and pushes them to [Sanoy24/homebrew-tap](https://github.com/Sanoy24/homebrew-tap) and [Sanoy24/scoop-bucket](https://github.com/Sanoy24/scoop-bucket). It needs a `PACKAGES_TOKEN` repository secret: a fine-grained personal access token with **Contents: Read and write** on those two repositories. Without the secret the job is skipped; to update them by hand, run `scripts/render-packages.sh 1.3.0 out` and copy `out/Formula/ytgrab.rb` and `out/bucket/ytgrab.json` into the two repositories.

To build the archives locally instead, put `yt-dlp.exe` and `SHA2-256SUMS` from the [yt-dlp releases](https://github.com/yt-dlp/yt-dlp/releases) in `tools/` and run on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\package.ps1 -Version 1.3.0 -AllPlatforms
```

## Further reading

- [Architecture](ARCHITECTURE.md)
- [Local API](API.md)
- [User guide](USER_GUIDE.md)

## Dependency updates

Dependabot ([`.github/dependabot.yml`](../.github/dependabot.yml)) opens one grouped pull request a week each for Go modules, the web test tooling in `web/`, and GitHub Actions versions. They go through CI like any other change; merge them when the checks pass. yt-dlp isn't a dependency of the code: releases bundle the latest official build, and YTGrab keeps it up to date on users' machines.

## App icon

The icon is drawn by [`scripts/make-icon.py`](../scripts/make-icon.py) (needs Pillow) into `packaging/icon/`. Windows builds pick it up from `cmd/ytgrab/rsrc_windows_*.syso`, which Go links automatically. After changing the icon, regenerate both:

```sh
python scripts/make-icon.py
cd cmd/ytgrab && go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --out rsrc --icon ../../packaging/icon/ytgrab.ico --manifest cli --product-name YTGrab --file-description "YTGrab local YouTube downloader"
```
