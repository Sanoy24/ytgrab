# Contributing to YTGrab

Thanks for helping! Bug reports, ideas, and pull requests are all welcome.

## Reporting a problem

[Open an issue](https://github.com/Sanoy24/ytgrab/issues/new/choose) and include:

- your system (Windows, macOS, or Linux) and YTGrab version (`ytgrab --version`),
- the output of `ytgrab doctor`,
- what you did, what you expected, and what happened, including any error message.

Download failures are often caused by YouTube changes rather than YTGrab. Update yt-dlp first (the **Update** button in the tools panel, or `ytgrab setup --update-ytdlp`) and try again.

## Making changes

1. Read [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) to run YTGrab from source and preview the UI with sample data.
2. Keep changes focused, and add or update tests for behavior you change.
3. Run the checks before opening a pull request:
   ```sh
   go vet ./...
   go test ./...
   cd web && npm test && npx playwright test   # when you change the browser UI
   ```
4. Update the docs (`README.md`, `docs/`) and add a line under a new heading at the top of `CHANGELOG.md` when users would notice the change.

## Design principles

- Everything stays local: the server listens only on `127.0.0.1`, with no accounts or telemetry.
- User input never becomes a yt-dlp option; the server builds every command itself.
- Media goes straight from yt-dlp to disk, never through the server or browser.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how the pieces fit together.

## Releases

Maintainers release by adding a version section to `CHANGELOG.md` and pushing a tag such as `v1.3.0`; the release workflow builds, checksums, and publishes every platform.
