#!/usr/bin/env bash
# Install build dependencies for ics-ics-baby

set -euo pipefail

echo "Installing build dependencies for ics-ics-baby..."
echo ""

# Detect OS
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
else
    echo "Cannot detect OS"
    exit 1
fi

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    armv7l) GOARCH="armv6l" ;;
    *) echo "Warning: Unsupported architecture: $ARCH"; GOARCH="$ARCH" ;;
esac

# Check and install Go
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Checking Go installation..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

GO_MIN_VERSION="1.22"
GO_LATEST_VERSION="1.23.5"

if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    echo "✓ Go is already installed: $GO_VERSION"

    # Check if version is sufficient (basic check)
    if [[ "$GO_VERSION" < "$GO_MIN_VERSION" ]]; then
        echo "⚠ Warning: Go $GO_MIN_VERSION or later is recommended (you have $GO_VERSION)"
        echo "  Consider upgrading for best compatibility"
    fi
else
    echo "Go is not installed. Installing Go $GO_LATEST_VERSION..."
    echo ""

    case "$OS" in
        ubuntu|debian)
            echo "Choose installation method:"
            echo "  1) Official Go binary (recommended, latest version)"
            echo "  2) apt package manager (may be older version)"
            read -p "Enter choice [1]: " choice
            choice=${choice:-1}

            if [ "$choice" = "1" ]; then
                GO_TAR="go${GO_LATEST_VERSION}.linux-${GOARCH}.tar.gz"
                GO_URL="https://go.dev/dl/${GO_TAR}"

                echo "Downloading Go ${GO_LATEST_VERSION} for linux/${GOARCH}..."
                wget -q --show-progress "$GO_URL" -O "/tmp/${GO_TAR}"

                echo "Installing to /usr/local/go..."
                sudo rm -rf /usr/local/go
                sudo tar -C /usr/local -xzf "/tmp/${GO_TAR}"
                rm "/tmp/${GO_TAR}"

                # Add to PATH if not already there
                if ! grep -q '/usr/local/go/bin' ~/.bashrc; then
                    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
                    echo "Added Go to PATH in ~/.bashrc"
                fi
                if ! grep -q '/usr/local/go/bin' ~/.profile 2>/dev/null; then
                    echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
                    echo "Added Go to PATH in ~/.profile"
                fi

                export PATH=$PATH:/usr/local/go/bin
                echo "✓ Go ${GO_LATEST_VERSION} installed successfully"
            else
                sudo apt-get update
                sudo apt-get install -y golang
                echo "✓ Go installed via apt"
            fi
            ;;
        fedora|rhel|centos)
            sudo dnf install -y golang || sudo yum install -y golang
            echo "✓ Go installed via package manager"
            ;;
        arch)
            sudo pacman -S --needed go
            echo "✓ Go installed via pacman"
            ;;
        *)
            echo "⚠ Cannot auto-install Go on $OS"
            echo "Please install Go manually from: https://go.dev/dl/"
            echo "Required version: Go $GO_MIN_VERSION or later"
            exit 1
            ;;
    esac

    # Verify installation
    if command -v go &> /dev/null; then
        echo "✓ Go version: $(go version | awk '{print $3}')"
    else
        echo "✗ Go installation failed or not in PATH"
        echo "  You may need to restart your shell or run: source ~/.bashrc"
        echo "  Then verify with: go version"
    fi
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Installing build dependencies (libseccomp)..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

case "$OS" in
    ubuntu|debian)
        echo "Detected Debian/Ubuntu"
        echo "Installing libseccomp-dev and build essentials..."
        sudo apt-get update
        sudo apt-get install -y libseccomp-dev build-essential pkg-config
        ;;
    fedora|rhel|centos)
        echo "Detected Fedora/RHEL/CentOS"
        echo "Installing libseccomp-devel and build tools..."
        sudo dnf install -y libseccomp-devel gcc gcc-c++ make || \
        sudo yum install -y libseccomp-devel gcc gcc-c++ make
        ;;
    arch)
        echo "Detected Arch Linux"
        echo "Installing libseccomp and build tools..."
        sudo pacman -S --needed libseccomp base-devel
        ;;
    *)
        echo "Unsupported OS: $OS"
        echo "Please manually install:"
        echo "  - libseccomp development files"
        echo "  - GCC compiler"
        echo "  - pkg-config"
        exit 1
        ;;
esac

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Verifying installation..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Verify Go
if command -v go &> /dev/null; then
    echo "✓ Go: $(go version | awk '{print $3}')"
else
    echo "✗ Go not found (you may need to restart your shell)"
fi

# Verify GCC
if command -v gcc &> /dev/null; then
    echo "✓ GCC: $(gcc --version | head -n1)"
else
    echo "✗ GCC not found"
fi

# Verify libseccomp
export PKG_CONFIG_PATH="${PKG_CONFIG_PATH:+${PKG_CONFIG_PATH}:}/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/lib/aarch64-linux-gnu/pkgconfig"
if pkg-config --exists libseccomp 2>/dev/null; then
    echo "✓ libseccomp: $(pkg-config --modversion libseccomp)"
else
    echo "✗ libseccomp not found via pkg-config"
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✓ All dependencies installed successfully"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Next steps:"
echo "  Build and install locally:  ./build.sh install"
echo "  Build distribution packages: ./build.sh"
echo "  Build wrapper only:         ./build-wrapper.sh"
echo ""
if ! command -v go &> /dev/null; then
    echo "⚠ Note: If Go was just installed, restart your shell first:"
    echo "  source ~/.bashrc"
    echo ""
fi
