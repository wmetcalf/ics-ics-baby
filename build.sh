#!/usr/bin/env bash
set -euo pipefail
shopt -s nullglob

APP=ics-ics-baby
MAIN=./cmd/ics-ics-baby
WRAPPER_MAIN=./cmd/wkhtml-wrap

# Version info
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

LDFLAGS="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE}"

DIST=dist

# Install mode: ./build.sh install [PREFIX]
if [ "${1:-}" = "install" ]; then
  PREFIX="${2:-$HOME/.local}"
  BINDIR="${PREFIX}/bin"

  # Detect current architecture
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
  esac

  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  if [ "$OS" != "linux" ] && [ "$OS" != "darwin" ]; then
    echo "Unsupported OS: $OS"
    exit 1
  fi

  echo "Building for local installation (${OS}/${GOARCH})..."
  echo ""

  # Build main binary
  echo "==> Building ${APP}"
  CGO_ENABLED=0 GOOS="$OS" GOARCH="$GOARCH" go build -trimpath -ldflags "${LDFLAGS}" -o "/tmp/${APP}" "${MAIN}"

  echo "Installing to ${BINDIR}/"
  install -d "${BINDIR}"
  install -m 755 "/tmp/${APP}" "${BINDIR}/${APP}"
  echo "✓ Installed ${APP} -> ${BINDIR}/${APP}"

  # Build and install wrapper (Linux only, requires libseccomp)
  if [ "$OS" = "linux" ]; then
    export PKG_CONFIG_PATH="${PKG_CONFIG_PATH:+${PKG_CONFIG_PATH}:}/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/lib/aarch64-linux-gnu/pkgconfig"

    if pkg-config --exists libseccomp 2>/dev/null; then
      echo ""
      echo "==> Building wkhtml-wrap (requires libseccomp)"
      (
        cd cmd/wkhtml-wrap
        CGO_ENABLED=1 GOOS=linux GOARCH="${GOARCH}" \
          go build -trimpath -ldflags "-s -w" -o "/tmp/wkhtml-wrap"
      )
      install -m 755 "/tmp/wkhtml-wrap" "${BINDIR}/wkhtml-wrap"
      echo "✓ Installed wkhtml-wrap -> ${BINDIR}/wkhtml-wrap"
      echo ""
      echo "Note: wkhtml-wrap requires libseccomp runtime library."
      echo "If not installed: sudo apt-get install -y libseccomp2"
    else
      echo ""
      echo "⚠ libseccomp-dev not found, skipping wkhtml-wrap"
      echo "  To enable secure sandboxing, install dependencies:"
      echo "  $ ./install-deps.sh"
      echo "  Then re-run: ./build.sh install"
    fi
  fi

  echo ""
  echo "Installation complete. Binaries available in PATH if ${BINDIR} is in PATH."
  exit 0
fi

# Build matrix
platforms=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

DIST=dist
rm -rf "${DIST}"
mkdir -p "${DIST}"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Note: wkhtml-wrap is NOT included in distribution packages"
echo "      It must be built locally on the target system (requires libseccomp)"
echo ""
echo "      To build and install locally: ./build.sh install"
echo "      Or build wrapper only: ./build-wrapper.sh"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

build_one() {
  local os="$1"; local arch="$2"
  local ext=""
  [ "$os" = "windows" ] && ext=".exe"
  local out="${DIST}/${APP}_${os}_${arch}${ext}"
  echo "==> Building ${out}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "${LDFLAGS}" -o "${out}" "${MAIN}"
}

# Ensure deps
go mod tidy

for p in "${platforms[@]}"; do
  os="${p%/*}"; arch="${p#*/}"
  build_one "$os" "$arch"
done

echo "All artifacts in ${DIST}/"
