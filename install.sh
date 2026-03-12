#!/bin/bash
set -e

REPO="smazurov/pinquake"
BIN_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.config/pinquake"
SYSTEMD_DIR="$HOME/.config/systemd/user"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() {
    echo -e "${GREEN}$1${NC}"
}

warn() {
    echo -e "${YELLOW}$1${NC}"
}

error() {
    echo -e "${RED}$1${NC}" >&2
}

# Parse flags
CHANNEL="latest"
for arg in "$@"; do
    case "$arg" in
        --dev) CHANNEL="dev" ;;
    esac
done
if [[ "${DEV:-0}" == "1" ]]; then
    CHANNEL="dev"
fi

UPGRADE=false
if [[ -x "$BIN_DIR/pinquake" ]]; then
    UPGRADE=true
fi

if $UPGRADE; then
    info "Upgrading pinquake (channel: $CHANNEL)..."
else
    info "Installing pinquake (channel: $CHANNEL)..."
fi
echo ""

# Step 1: Detect architecture
info "[1/3] Detecting architecture..."
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        error "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac
echo "      $ARCH"

# Step 2: Download and install binary
info "[2/3] Downloading pinquake..."
if [[ "$CHANNEL" == "dev" ]]; then
    DOWNLOAD_URL="https://github.com/$REPO/releases/download/dev/pinquake_linux_${ARCH}.tar.gz"
else
    DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/pinquake_linux_${ARCH}.tar.gz"
fi
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

if ! curl -fsSL -o "$TEMP_DIR/pinquake.tar.gz" "$DOWNLOAD_URL"; then
    error "Failed to download from $DOWNLOAD_URL"
    error "Make sure a release exists with the archive: pinquake_linux_${ARCH}.tar.gz"
    exit 1
fi

if $UPGRADE; then
    if command -v systemctl &> /dev/null && systemctl --user is-active pinquake.service &> /dev/null; then
        echo "      Stopping pinquake service..."
        systemctl --user stop pinquake.service
    fi
fi

mkdir -p "$BIN_DIR"
tar -xzf "$TEMP_DIR/pinquake.tar.gz" -C "$TEMP_DIR"
mv "$TEMP_DIR/pinquake" "$BIN_DIR/pinquake"
chmod +x "$BIN_DIR/pinquake"
echo "      Installed to $BIN_DIR/pinquake"

# Step 3: Systemd service
info "[3/3] Setting up systemd service..."
mkdir -p "$CONFIG_DIR"

if command -v systemctl &> /dev/null && systemctl --user status 2>/dev/null; then
    mkdir -p "$SYSTEMD_DIR"

    cat > "$SYSTEMD_DIR/pinquake.service" << EOF
[Unit]
Description=Pinquake sensor server
After=network-online.target bluetooth.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$CONFIG_DIR
ExecStart=$BIN_DIR/pinquake
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

    systemctl --user daemon-reload

    if $UPGRADE; then
        echo "      Updated $SYSTEMD_DIR/pinquake.service"
    else
        systemctl --user enable pinquake.service 2>/dev/null || true
        echo "      Enabled pinquake.service"
    fi
else
    warn "      Systemd user services not available, skipping"
fi

echo ""
if $UPGRADE; then
    if command -v systemctl &> /dev/null && systemctl --user is-enabled pinquake.service &> /dev/null; then
        systemctl --user start pinquake.service
        echo "      Service restarted"
        echo ""
    fi
    info "Upgrade complete!"
else
    info "Installation complete!"
    echo ""
    echo "To start pinquake now:"
    echo "  systemctl --user start pinquake"
    echo ""
    echo "To view logs:"
    echo "  journalctl --user -u pinquake -f"
    echo ""
    echo "Config files: $CONFIG_DIR/"

    if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
        echo ""
        warn "Warning: $BIN_DIR is not in your PATH"
        warn "Add this to your shell profile (~/.bashrc or ~/.zshrc):"
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    fi
fi
echo ""
