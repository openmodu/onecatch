#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
VERSION=$(tr -d '[:space:]' < "$REPO_ROOT/VERSION")

if ! command -v makensis >/dev/null 2>&1; then
    echo "error: makensis is required; on macOS run: brew install makensis" >&2
    exit 1
fi
if ! printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "error: VERSION must use X.Y.Z numeric format, got: $VERSION" >&2
    exit 1
fi

STAGING_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/onecatch-nsis-validation.XXXXXX")
cleanup() {
    rm -rf "$STAGING_ROOT"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$STAGING_ROOT/windows/nsis"
install -m 0644 "$SCRIPT_DIR/icon.ico" "$STAGING_ROOT/windows/icon.ico"
install -m 0644 "$SCRIPT_DIR/nsis/project.nsi" "$STAGING_ROOT/windows/nsis/project.nsi"

# NSIS only needs readable payloads to validate the installer definition. The
# VERSION file keeps this check fast and avoids cross-compiling Windows binaries.
install -m 0644 "$REPO_ROOT/VERSION" "$STAGING_ROOT/payload"
install -m 0644 "$REPO_ROOT/VERSION" "$STAGING_ROOT/windows/nsis/MicrosoftEdgeWebview2Setup.exe"

makensis \
    -WX \
    "-DAPP_VERSION=$VERSION" \
    -DAPP_ARCH=x64 \
    "-DAPP_BINARY=$STAGING_ROOT/payload" \
    "-DWORKER_BINARY=$STAGING_ROOT/payload" \
    "-DASKPASS_BINARY=$STAGING_ROOT/payload" \
    "-DUPDATER_BINARY=$STAGING_ROOT/payload" \
    "-DOUTPUT_FILE=$STAGING_ROOT/OneCatch-validation-Setup.exe" \
    "$STAGING_ROOT/windows/nsis/project.nsi"

test -f "$STAGING_ROOT/OneCatch-validation-Setup.exe"
echo "Windows installer definition: passed"
