#!/bin/sh
# Installs YTGrab on macOS or Linux:
#
#   curl -fsSL https://raw.githubusercontent.com/Sanoy24/ytgrab/main/install.sh | sh
#
# Downloads the release for this system, verifies its SHA-256 checksum, installs it into
# ~/.local/share/ytgrab, and adds a `ytgrab` command to ~/.local/bin. Files fetched with
# curl are not quarantined, so macOS does not block the unsigned program.
#
# Environment variables:
#   YTGRAB_VERSION      version to install, for example 1.0.0 (default: latest release)
#   YTGRAB_INSTALL_DIR  program folder (default: ~/.local/share/ytgrab)
#   YTGRAB_BIN_DIR      folder for the `ytgrab` command (default: ~/.local/bin)
#   YTGRAB_NO_SETUP=1   don't offer to run `ytgrab setup` afterwards
#   YTGRAB_NO_MENU=1    on Linux, don't add YTGrab to the applications menu

set -eu

REPO="Sanoy24/ytgrab"
BASE_URL="${YTGRAB_DOWNLOAD_URL:-https://github.com/$REPO/releases}"
INSTALL_DIR="${YTGRAB_INSTALL_DIR:-$HOME/.local/share/ytgrab}"
BIN_DIR="${YTGRAB_BIN_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }

need() {
    command -v "$1" >/dev/null 2>&1 || fail "$1 is required but was not found."
}

need curl
need tar
need uname
need mktemp

case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux) os=linux ;;
    *) fail "this installer supports macOS and Linux. On Windows, download the zip from https://github.com/$REPO/releases" ;;
esac

case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    arm64 | aarch64) arch=arm64 ;;
    *) arch="$(uname -m)" ;;
esac

case "$arch" in
    amd64 | arm64) ;;
    *) fail "there is no prebuilt $os/$arch release. Install with Go instead: go install github.com/$REPO/cmd/ytgrab@latest" ;;
esac
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
    arch=arm64 # running under Rosetta on Apple silicon; use the native build
fi

if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
    fail "sha256sum or shasum is required to verify the download."
fi

version="${YTGRAB_VERSION:-}"
if [ -z "$version" ]; then
    # The latest-release page redirects to .../releases/tag/vX.Y.Z.
    latest="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE_URL/latest")" ||
        fail "could not reach GitHub to find the latest release."
    version="${latest##*/}"
fi
version="${version#v}"
case "$version" in
    '' | *[!0-9A-Za-z.-]*) fail "could not determine which version to install (got '$version')." ;;
esac

archive="ytgrab-$version-$os-$arch.tar.gz"
url="$BASE_URL/download/v$version/$archive"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

say "Downloading YTGrab $version for $os/$arch..."
curl -fsSL -o "$tmp/$archive" "$url" || fail "could not download $url"
curl -fsSL -o "$tmp/$archive.sha256" "$url.sha256" || fail "could not download the checksum for $archive"

expected="$(cut -d ' ' -f 1 <"$tmp/$archive.sha256")"
actual="$(sha256 "$tmp/$archive")"
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
    fail "checksum mismatch for $archive (expected $expected, got $actual). Nothing was installed."
fi
say "Checksum verified."

mkdir -p "$tmp/extract"
tar -xzf "$tmp/$archive" -C "$tmp/extract"
[ -f "$tmp/extract/ytgrab" ] || fail "the archive does not contain the ytgrab program."

# Replace only the program; tools/ (yt-dlp from `ytgrab setup`) is kept across upgrades.
mkdir -p "$INSTALL_DIR" "$BIN_DIR"
cp "$tmp/extract/ytgrab" "$INSTALL_DIR/ytgrab.new"
chmod 755 "$INSTALL_DIR/ytgrab.new"
if [ "$os" = darwin ]; then
    xattr -c "$INSTALL_DIR/ytgrab.new" 2>/dev/null || true
fi
mv -f "$INSTALL_DIR/ytgrab.new" "$INSTALL_DIR/ytgrab"
for doc in README.md LICENSE.txt ytgrab.png; do
    if [ -f "$tmp/extract/$doc" ]; then cp "$tmp/extract/$doc" "$INSTALL_DIR/$doc"; fi
done

# A launcher rather than a symlink, so the program always sees its real folder.
launcher="$BIN_DIR/ytgrab"
cat >"$launcher" <<EOF
#!/bin/sh
exec "$INSTALL_DIR/ytgrab" "\$@"
EOF
chmod 755 "$launcher"

installed="$("$INSTALL_DIR/ytgrab" --version 2>/dev/null)" || fail "the installed program did not start."
say "Installed YTGrab $installed to $INSTALL_DIR"
say "The ytgrab command is in $BIN_DIR"

# On Linux, add YTGrab to the applications menu. Started from there it has no terminal;
# its tray icon (on desktops with one) opens and quits it.
menu_entry=""
if [ "$os" = linux ] && [ "${YTGRAB_NO_MENU:-}" != 1 ]; then
    case "$INSTALL_DIR" in
        *[\\\"\`\$]*) say "Not adding a menu entry: the install folder's name has characters a menu entry can't hold." ;;
        *)
            apps="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
            mkdir -p "$apps"
            menu_entry="$apps/ytgrab.desktop"
            {
                printf '[Desktop Entry]\n'
                printf 'Type=Application\n'
                printf 'Name=YTGrab\n'
                printf 'GenericName=YouTube downloader\n'
                printf 'Comment=Download YouTube videos you are allowed to save\n'
                printf 'Exec="%s" --open\n' "$INSTALL_DIR/ytgrab"
                if [ -f "$INSTALL_DIR/ytgrab.png" ]; then printf 'Icon=%s\n' "$INSTALL_DIR/ytgrab.png"; fi
                printf 'Terminal=false\n'
                printf 'Categories=Network;AudioVideo;\n'
                printf 'StartupNotify=false\n'
            } >"$menu_entry"
            if command -v update-desktop-database >/dev/null 2>&1; then
                update-desktop-database "$apps" 2>/dev/null || true
            fi
            say "Added YTGrab to your applications menu"
            ;;
    esac
fi

case ":$PATH:" in
    *":$BIN_DIR:"*) on_path=1 ;;
    *) on_path=0 ;;
esac
if [ "$on_path" = 0 ]; then
    case "${SHELL:-}" in
        */zsh) rc="$HOME/.zshrc" ;;
        */bash) rc="$HOME/.bashrc" ;;
        *) rc="your shell's startup file" ;;
    esac
    say ""
    say "$BIN_DIR is not on your PATH. Add it by running:"
    say "  echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> $rc"
    say "then open a new terminal. Until then, run $launcher directly."
fi

run_setup=0
if [ "${YTGRAB_NO_SETUP:-}" != 1 ] && [ -r /dev/tty ] && (exec </dev/tty) 2>/dev/null; then
    printf '\nRun "ytgrab setup" now to install yt-dlp, FFmpeg, and Deno if they are missing? [Y/n] '
    read -r answer </dev/tty || answer=n
    case "$answer" in '' | [Yy] | [Yy][Ee][Ss]) run_setup=1 ;; esac
fi
if [ "$run_setup" = 1 ]; then
    say ""
    "$INSTALL_DIR/ytgrab" setup </dev/tty || true
fi

say ""
say "Start YTGrab with:  ytgrab --open"
say "Check your setup:   ytgrab doctor"
if [ -n "$menu_entry" ]; then
    say "Uninstall:          rm -rf \"$INSTALL_DIR\" \"$launcher\" \"$menu_entry\""
else
    say "Uninstall:          rm -rf \"$INSTALL_DIR\" \"$launcher\""
fi
