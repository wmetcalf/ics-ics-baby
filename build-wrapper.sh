#!/usr/bin/env bash
# Build script for wkhtml-wrap secure sandbox wrapper
# This wrapper provides sandboxing for wkhtmltoimage using seccomp on Linux

set -euo pipefail

echo "Building wkhtml-wrap secure sandbox wrapper..."
echo ""

# Set PKG_CONFIG_PATH to include standard locations
export PKG_CONFIG_PATH="${PKG_CONFIG_PATH:+${PKG_CONFIG_PATH}:}/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/lib/aarch64-linux-gnu/pkgconfig"

# Check if libseccomp-dev is installed
if ! pkg-config --exists libseccomp 2>/dev/null; then
    echo "ERROR: libseccomp-dev is not installed."
    echo ""
    echo "Install dependencies with:"
    echo "  ./install-deps.sh"
    echo ""
    echo "Or manually:"
    echo "  sudo apt-get update"
    echo "  sudo apt-get install -y libseccomp-dev"
    echo ""
    exit 1
fi

echo "libseccomp found: $(pkg-config --modversion libseccomp)"
echo ""

# Build the wrapper
echo "==> Building wkhtml-wrap"
(
  cd cmd/wkhtml-wrap
  CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o ../../wkhtml-wrap
)

if [ -f ./wkhtml-wrap ]; then
    echo ""
    echo "✓ Built wkhtml-wrap successfully"
    echo ""
    echo "Install options:"
    echo "  User install:   install -d ~/.local/bin && install -m 755 ./wkhtml-wrap ~/.local/bin/"
    echo "  System install: sudo install -m 755 ./wkhtml-wrap /usr/local/bin/"
    echo ""
    echo "Runtime requirement:"
    echo "  sudo apt-get install -y libseccomp2"
    echo ""
else
    echo "ERROR: Build failed"
    exit 1
fi
