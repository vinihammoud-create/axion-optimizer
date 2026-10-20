#!/usr/bin/env bash
# Builds dist/Axion.exe (Windows amd64) with manifest, icon and version info
# (the resources come from cmd/axion-rsrc; no llvm-windres needed).
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${1:-0.9.1}"
export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off CGO_ENABLED=0
go run ./cmd/axion-rsrc -version "$VERSION"
go vet ./...
GOOS=windows GOARCH=amd64 go vet ./cmd/... ./internal/...
go test ./internal/...
mkdir -p dist
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui -X main.Version=$VERSION" -o dist/Axion.exe ./cmd/axion
ls -la dist/Axion.exe

# Package
PKG="dist/Axion-$VERSION-win64"
rm -rf "$PKG" "$PKG.zip"
mkdir -p "$PKG"
cp dist/Axion.exe "$PKG/"
# The WebView2 loader (signed by Microsoft) ships next to the exe; its hash
# is pinned in cmd/axion/main_windows.go.
WV2=third_party/go-webview2/webviewloader/sdk/x64/WebView2Loader.dll
grep -q "$(sha256sum "$WV2" | cut -d' ' -f1)" cmd/axion/main_windows.go || { echo "hash do WebView2Loader.dll não confere com o código"; exit 1; }
cp "$WV2" "$PKG/"
# PresentMon (Intel, MIT, signed release) for the Desempenho page; its hash is
# pinned in internal/app/perf.go.
PM=third_party/presentmon/PresentMon-2.6.0-x64.exe
grep -q "$(sha256sum "$PM" | cut -d' ' -f1)" internal/app/perf.go || { echo "hash do PresentMon não confere com o código"; exit 1; }
cp "$PM" "$PKG/"
sed "s/@VERSION@/$VERSION/g" build/LEIA-ME.txt | sed 's/$/\r/' > "$PKG/LEIA-ME.txt"
go run ./cmd/axion-matrix > "$PKG/MATRIZ-DE-EVIDENCIAS.md"
sed 's/$/\r/' build/THIRD-PARTY-NOTICES.txt > "$PKG/THIRD-PARTY-NOTICES.txt"
sed 's/$/\r/' LICENSE > "$PKG/LICENSE.txt"
( cd "$PKG" && sha256sum Axion.exe WebView2Loader.dll PresentMon-2.6.0-x64.exe > SHA256SUMS.txt )
( cd dist && rm -f "Axion-$VERSION-win64.zip" && python3 -c "
import zipfile, os, sys
base = 'Axion-$VERSION-win64'
with zipfile.ZipFile(base + '.zip', 'w', zipfile.ZIP_DEFLATED) as z:
    for f in sorted(os.listdir(base)):
        z.write(os.path.join(base, f), os.path.join(base, f))
" )
ls -la "$PKG" "dist/Axion-$VERSION-win64.zip"
cat "$PKG/SHA256SUMS.txt"
