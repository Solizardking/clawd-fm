#!/bin/sh
set -e

REPO="clawd-fi/clawdamp"

# Determine install directory: prefer ~/.local/bin (no sudo), fall back to /usr/local/bin
if [ -z "$INSTALL_DIR" ]; then
    LOCAL_BIN="$HOME/.local/bin"
    if echo "$PATH" | tr ':' '\n' | grep -qx "$LOCAL_BIN"; then
        mkdir -p "$LOCAL_BIN"
        INSTALL_DIR="$LOCAL_BIN"
    else
        INSTALL_DIR="/usr/local/bin"
    fi
fi

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    mingw*|msys*|cygwin*) OS="windows" ;;
    *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

BINARY="clawdamp-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
    BINARY="${BINARY}.exe"
fi

URL="https://github.com/${REPO}/releases/latest/download/${BINARY}"

echo "Downloading ${BINARY}..."
TMP=$(mktemp)
if command -v curl > /dev/null; then
    curl -fSL -o "$TMP" "$URL"
elif command -v wget > /dev/null; then
    wget -qO "$TMP" "$URL"
else
    echo "Error: curl or wget required" >&2; exit 1
fi

chmod +x "$TMP"

# Verify checksum if checksums.txt is available
CHECKSUM_URL="https://github.com/${REPO}/releases/latest/download/checksums.txt"
CHECKSUMS=$(mktemp)
GOT_CHECKSUMS=false
if command -v curl > /dev/null; then
    curl -fSL -o "$CHECKSUMS" "$CHECKSUM_URL" 2>/dev/null && GOT_CHECKSUMS=true
elif command -v wget > /dev/null; then
    wget -qO "$CHECKSUMS" "$CHECKSUM_URL" 2>/dev/null && GOT_CHECKSUMS=true
fi

if [ "$GOT_CHECKSUMS" = true ]; then
    EXPECTED=$(grep "${BINARY}$" "$CHECKSUMS" | awk '{print $1}')
    if [ -n "$EXPECTED" ]; then
        if command -v sha256sum > /dev/null; then
            ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
        elif command -v shasum > /dev/null; then
            ACTUAL=$(shasum -a 256 "$TMP" | awk '{print $1}')
        else
            ACTUAL=""
        fi
        if [ -n "$ACTUAL" ] && [ "$ACTUAL" != "$EXPECTED" ]; then
            echo "Error: checksum mismatch" >&2
            echo "  expected: $EXPECTED" >&2
            echo "  got:      $ACTUAL" >&2
            rm -f "$TMP" "$CHECKSUMS"
            exit 1
        fi
        if [ -n "$ACTUAL" ]; then
            echo "Checksum verified."
        fi
    fi
fi
rm -f "$CHECKSUMS"

if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "${INSTALL_DIR}/clawdamp"
else
    sudo mv "$TMP" "${INSTALL_DIR}/clawdamp"
fi

echo "Installed clawdamp to ${INSTALL_DIR}/clawdamp"

# Install desktop entry + icon (Linux only).
# Pick user-local share dir for ~/.local/bin installs, /usr/local/share otherwise.
if [ "$OS" = "linux" ]; then
    case "$INSTALL_DIR" in
        "$HOME"/*)
            SHARE_DIR="${XDG_DATA_HOME:-$HOME/.local/share}"
            SUDO=""
            ;;
        *)
            SHARE_DIR="/usr/local/share"
            SUDO="sudo"
            [ -w "$SHARE_DIR" ] && SUDO=""
            ;;
    esac

    APP_DIR="${SHARE_DIR}/applications"
    ICON_DIR="${SHARE_DIR}/icons/hicolor/512x512/apps"

    ICON_TMP=$(mktemp)
    DESKTOP_TMP=$(mktemp)
    ICON_URL="https://raw.githubusercontent.com/${REPO}/HEAD/Clawdamp.png"

    GOT_ICON=false
    if command -v curl > /dev/null; then
        curl -fSL -o "$ICON_TMP" "$ICON_URL" 2>/dev/null && GOT_ICON=true
    elif command -v wget > /dev/null; then
        wget -qO "$ICON_TMP" "$ICON_URL" 2>/dev/null && GOT_ICON=true
    fi

    cat > "$DESKTOP_TMP" <<EOF
[Desktop Entry]
Name=clawdamp
GenericName=Music Player
Comment=A retro terminal music player inspired by Winamp 2.x
Exec=${INSTALL_DIR}/clawdamp
Icon=clawdamp
Terminal=true
Type=Application
Categories=Audio;Music;Player;AudioVideo;ConsoleOnly;
Keywords=music;audio;player;terminal;tui;winamp;radio;podcast;
StartupNotify=false
EOF

    $SUDO mkdir -p "$APP_DIR" "$ICON_DIR"
    $SUDO install -m 0644 "$DESKTOP_TMP" "${APP_DIR}/clawdamp.desktop"
    if [ "$GOT_ICON" = true ]; then
        $SUDO install -m 0644 "$ICON_TMP" "${ICON_DIR}/clawdamp.png"
        echo "Installed icon to ${ICON_DIR}/clawdamp.png"
    fi
    rm -f "$ICON_TMP" "$DESKTOP_TMP"

    echo "Installed desktop entry to ${APP_DIR}/clawdamp.desktop"

    if command -v update-desktop-database > /dev/null; then
        $SUDO update-desktop-database "$APP_DIR" 2>/dev/null || true
    fi
    if command -v gtk-update-icon-cache > /dev/null; then
        $SUDO gtk-update-icon-cache -q "${SHARE_DIR}/icons/hicolor" 2>/dev/null || true
    fi
fi
