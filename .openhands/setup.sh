#!/bin/bash
# Runs when OpenHands starts working on this repository: installs the Go
# version the project builds with, checked against the official SHA-256.
# Best effort: a failure here must not stop the conversation.
set -u
GO_VERSION=1.27.4
GO_SHA256=7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840
DEST="$HOME/sdk/go$GO_VERSION"

if [ ! -x "$DEST/go/bin/go" ]; then
  tmp=$(mktemp -d)
  if curl -fsSL -o "$tmp/go.tgz" "https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz" &&
    echo "$GO_SHA256  $tmp/go.tgz" | sha256sum -c - >/dev/null 2>&1; then
    mkdir -p "$DEST" && tar -C "$DEST" -xzf "$tmp/go.tgz"
  else
    echo "setup.sh: Go $GO_VERSION não foi instalado (download ou hash falhou)" >&2
  fi
  rm -rf "$tmp"
fi

if [ -x "$DEST/go/bin/go" ]; then
  line="export PATH=\"$DEST/go/bin:\$PATH\" GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off CGO_ENABLED=0 GOTOOLCHAIN=local"
  grep -qsF "$DEST/go/bin" "$HOME/.bashrc" || echo "$line" >>"$HOME/.bashrc"
  for d in /usr/local/bin "$HOME/.local/bin"; do
    mkdir -p "$d" 2>/dev/null
    if [ -w "$d" ]; then
      ln -sf "$DEST/go/bin/go" "$d/go" && ln -sf "$DEST/go/bin/gofmt" "$d/gofmt" && break
    fi
  done
  "$DEST/go/bin/go" version
fi
exit 0
