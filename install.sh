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

usage() {
    cat << 'EOF'
Usage: install.sh [version]

  version   Release to install: a version number (1.4.0 or v1.4.0), or "dev"
            for the rolling build from main. Leave it out for the latest release.
            Upgrades and downgrades both work.

Examples:
  install.sh                latest release
  install.sh 1.4.0          a specific version
  install.sh dev            rolling dev build

  curl -fsSL https://raw.githubusercontent.com/smazurov/pinquake/main/install.sh \
    | bash -s -- 1.4.0

  PINQUAKE_VERSION=1.4.0    the same thing through the environment
EOF
}

# The version to install. "dev" is a version like any other: it names the rolling
# release that main pushes to.
VERSION="${PINQUAKE_VERSION:-}"
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h | --help)
            usage
            exit 0
            ;;
        -*)
            error "Unknown option: $1"
            usage >&2
            exit 1
            ;;
        *)
            if [[ -n "$VERSION" && "$VERSION" != "$1" ]]; then
                error "Specify only one version (got \"$VERSION\" and \"$1\")"
                exit 1
            fi
            VERSION="$1"
            ;;
    esac
    shift
done

# Drop an optional leading "v", then validate.
VERSION="${VERSION#v}"
if [[ -n "$VERSION" && "$VERSION" != "dev" ]]; then
    if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
        error "Invalid version: \"$VERSION\". Expected something like 1.4.0, or \"dev\"."
        exit 1
    fi
fi

# Release tag the archive lives under, empty for "whatever is latest".
if [[ "$VERSION" == "dev" ]]; then
    TAG="dev"
    LABEL="dev build"
elif [[ -n "$VERSION" ]]; then
    TAG="v$VERSION"
    LABEL="$TAG"
else
    TAG=""
    LABEL="latest release"
fi

# installed_version echoes the version of the installed binary, or fails when there
# is none. Binaries built before --version existed fail too.
installed_version() {
    local bin="$BIN_DIR/pinquake" out
    [[ -x "$bin" ]] || return 1
    out=$("$bin" --version 2>/dev/null | tr -d '[:space:]') || return 1
    [[ -n "$out" ]] || return 1
    echo "${out#v}"
}

# version_cmp echoes -1, 0, or 1 for $1 less than, equal to, or greater than $2.
version_cmp() {
    local lowest
    if [[ "$1" == "$2" ]]; then
        echo 0
        return
    fi
    lowest=$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)
    if [[ "$lowest" == "$1" ]]; then
        echo -1
    else
        echo 1
    fi
}

INSTALLED=$(installed_version || true)
UPGRADE=false
if [[ -x "$BIN_DIR/pinquake" ]]; then
    UPGRADE=true
fi

if $UPGRADE; then
    info "Updating pinquake to $LABEL..."
    if [[ -n "$VERSION" && "$VERSION" != "dev" ]]; then
        case "$(version_cmp "$VERSION" "$INSTALLED")" in
            -1)
                warn "This is a downgrade: $INSTALLED -> $VERSION"
                warn "Config written by the newer version may not be understood."
                ;;
            0) info "$INSTALLED is already installed, reinstalling." ;;
        esac
    fi
else
    info "Installing pinquake ($LABEL)..."
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
if [[ -n "$TAG" ]]; then
    DOWNLOAD_URL="https://github.com/$REPO/releases/download/$TAG/pinquake_linux_${ARCH}.tar.gz"
else
    DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/pinquake_linux_${ARCH}.tar.gz"
fi
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

if ! curl -fsSL -o "$TEMP_DIR/pinquake.tar.gz" "$DOWNLOAD_URL"; then
    error "Failed to download from $DOWNLOAD_URL"
    if [[ -n "$TAG" ]]; then
        error "Release $TAG not found. Available releases:"
        error "  https://github.com/$REPO/releases"
    else
        error "Make sure a release exists with the archive: pinquake_linux_${ARCH}.tar.gz"
    fi
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

NEW=$(installed_version || true)
if [[ -n "$NEW" ]]; then
    echo "      Installed $NEW to $BIN_DIR/pinquake"
else
    echo "      Installed to $BIN_DIR/pinquake"
fi

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
    if [[ -n "$INSTALLED" && -n "$NEW" ]]; then
        case "$(version_cmp "$NEW" "$INSTALLED")" in
            -1) info "Downgraded pinquake $INSTALLED -> $NEW" ;;
            1)  info "Upgraded pinquake $INSTALLED -> $NEW" ;;
            *)  info "Reinstalled pinquake $NEW" ;;
        esac
    else
        info "Update complete!"
    fi
    if command -v systemctl &> /dev/null && systemctl --user is-enabled pinquake.service &> /dev/null; then
        systemctl --user start pinquake.service
        echo "      Service restarted"
    fi
else
    if [[ -n "$NEW" ]]; then
        info "Installed pinquake $NEW!"
    else
        info "Installation complete!"
    fi
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
