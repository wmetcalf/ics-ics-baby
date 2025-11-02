#!/usr/bin/env bash
set -euo pipefail
shopt -s nullglob

APP=ics-ics-baby
MAIN=./cmd/ics-ics-baby

# Version info
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

LDFLAGS="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.Date=${DATE}"

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

build_one() {
  local os="$1"; local arch="$2"
  local ext=""
  [ "$os" = "windows" ] && ext=".exe"
  local out="${DIST}/${APP}_${os}_${arch}${ext}"
  echo "==> Building ${out}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "${LDFLAGS}" -o "${out}" "${MAIN}"

  # Package (zip for windows, tar.gz otherwise)
  local pkgbase="${APP}_${VERSION}_${os}_${arch}"
  local pkgdir="${DIST}/${pkgbase}"
  mkdir -p "${pkgdir}"
  cp -f README.md "${pkgdir}/README.md" || true
  cp -f LICENSE "${pkgdir}/LICENSE" || true
  cp -f "${out}" "${pkgdir}/${APP}${ext}"

  if [ "$os" = "windows" ]; then
    (cd "${DIST}" && zip -r "${pkgbase}.zip" "${pkgbase}")
    rm -rf "${pkgdir}"
  else
    (cd "${DIST}" && tar -czf "${pkgbase}.tar.gz" "${pkgbase}")
    rm -rf "${pkgdir}"
  fi
}

# Ensure deps
go mod tidy

for p in "${platforms[@]}"; do
  os="${p%/*}"; arch="${p#*/}"
  build_one "$os" "$arch"
done

echo "==> Checksums"
(cd "${DIST}" && sha256sum * > SHA256SUMS || shasum -a 256 * > SHA256SUMS)
(cd "${DIST}" && md5sum * > MD5SUMS || md5 * > MD5SUMS || true)

echo "All artifacts in ${DIST}/"
