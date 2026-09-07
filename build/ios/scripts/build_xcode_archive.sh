#!/bin/sh
set -eu

go_bin=$(command -v go 2>/dev/null || true)
if [ -z "$go_bin" ]; then
  for candidate in \
    "/etc/profiles/per-user/${USER:-}/bin/go" \
    /opt/homebrew/bin/go \
    /usr/local/bin/go \
    /usr/local/go/bin/go \
    /run/current-system/sw/bin/go \
    /nix/var/nix/profiles/default/bin/go
  do
    if [ -x "$candidate" ]; then
      go_bin=$candidate
      break
    fi
  done
fi
if [ -z "$go_bin" ]; then
  echo "Go executable not found. Install Go or make it available to Xcode." >&2
  exit 1
fi

# Xcode already exports GOOS, GOARCH and the SDK-specific CGO flags in the
# generated build phase. Keep the package and custom tags aligned with the
# task-based iOS build; a bare `go build` at the repository root has no package.
if [ ! -f build/ios/xcode/overlay.json ]; then
  "$go_bin" tool wails3 ios overlay:gen \
    -out build/ios/xcode/overlay.json \
    -config build/desktop/config.yml
fi

if [ "${CONFIGURATION:-Debug}" = "Release" ]; then
  "$go_bin" build \
    -tags production,ios \
    -trimpath \
    -buildvcs=false \
    -buildmode=c-archive \
    -overlay build/ios/xcode/overlay.json \
    -o bin/OneCatch.a \
    ./cmd/app
else
  "$go_bin" build \
    -tags ios,debug \
    -buildvcs=false \
    -gcflags=all=-l \
    -buildmode=c-archive \
    -overlay build/ios/xcode/overlay.json \
    -o bin/OneCatch.a \
    ./cmd/app
fi
