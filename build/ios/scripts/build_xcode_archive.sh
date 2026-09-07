#!/bin/sh
set -eu

# Xcode already exports GOOS, GOARCH and the SDK-specific CGO flags in the
# generated build phase. Keep the package and custom tags aligned with the
# task-based iOS build; a bare `go build` at the repository root has no package.
if [ "${CONFIGURATION:-Debug}" = "Release" ]; then
  go build \
    -tags production,ios \
    -trimpath \
    -buildvcs=false \
    -buildmode=c-archive \
    -overlay build/ios/xcode/overlay.json \
    -o bin/OneCatch.a \
    ./cmd/app
else
  go build \
    -tags ios,debug \
    -buildvcs=false \
    -gcflags=all=-l \
    -buildmode=c-archive \
    -overlay build/ios/xcode/overlay.json \
    -o bin/OneCatch.a \
    ./cmd/app
fi
