#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
BIN_ROOT="$REPO_ROOT/bin"
VERSION_FILE="$REPO_ROOT/VERSION"
APP_BINARY="$BIN_ROOT/onecatch"
WORKER_BINARY="$BIN_ROOT/onecatch-worker"
SHELL_BINARY="$BIN_ROOT/onecatchsh"
ASKPASS_BINARY="$BIN_ROOT/onecatch-askpass"
ICON_FILE="$REPO_ROOT/internal/app/desktop/assets/appicon.png"
DESKTOP_FILE="$SCRIPT_DIR/onecatch.desktop"
NFPM_CONFIG="$SCRIPT_DIR/nfpm.yaml"

for tool in curl go sha256sum; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "error: required command not found: $tool" >&2
        exit 1
    fi
done

for input in \
    "$VERSION_FILE" \
    "$APP_BINARY" \
    "$WORKER_BINARY" \
    "$SHELL_BINARY" \
    "$ASKPASS_BINARY" \
    "$ICON_FILE" \
    "$DESKTOP_FILE" \
    "$NFPM_CONFIG"; do
    if [ ! -f "$input" ]; then
        echo "error: required build input not found: $input" >&2
        exit 1
    fi
done

for executable in "$APP_BINARY" "$WORKER_BINARY" "$SHELL_BINARY" "$ASKPASS_BINARY"; do
    if [ ! -x "$executable" ]; then
        echo "error: built binary is not executable: $executable" >&2
        exit 1
    fi
done

VERSION=${VERSION:-$(tr -d '[:space:]' < "$VERSION_FILE")}
if ! printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "error: VERSION must use X.Y.Z numeric format, got: $VERSION" >&2
    exit 1
fi

GO_ARCH=$(go env GOARCH)
case "$GO_ARCH" in
    amd64)
        APPIMAGE_ARCH=x86_64
        ARTIFACT_ARCH=x64
        ;;
    arm64)
        APPIMAGE_ARCH=aarch64
        ARTIFACT_ARCH=arm64
        ;;
    *)
        echo "error: unsupported Linux architecture: $GO_ARCH" >&2
        exit 1
        ;;
esac

OUTPUT_APPIMAGE=${OUTPUT_APPIMAGE:-$BIN_ROOT/OneCatch-$VERSION-Linux-$ARTIFACT_ARCH.AppImage}
OUTPUT_DEB=${OUTPUT_DEB:-$BIN_ROOT/OneCatch-$VERSION-Linux-$ARTIFACT_ARCH.deb}
case "$OUTPUT_APPIMAGE" in
    /*) ;;
    *) OUTPUT_APPIMAGE="$REPO_ROOT/$OUTPUT_APPIMAGE" ;;
esac
case "$OUTPUT_DEB" in
    /*) ;;
    *) OUTPUT_DEB="$REPO_ROOT/$OUTPUT_DEB" ;;
esac
mkdir -p "$(dirname -- "$OUTPUT_APPIMAGE")" "$(dirname -- "$OUTPUT_DEB")"

STAGING_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/onecatch-linux-package.XXXXXX")
cleanup() {
    rm -rf "$STAGING_ROOT"
}
trap cleanup EXIT HUP INT TERM

STAGING_ICON="$STAGING_ROOT/onecatch.png"
APPIMAGE_OUTPUT="$STAGING_ROOT/output"
APPIMAGE_BUILD="$STAGING_ROOT/build"
install -m 0644 "$ICON_FILE" "$STAGING_ICON"
mkdir -p "$APPIMAGE_OUTPUT" "$APPIMAGE_BUILD"

cd "$REPO_ROOT"
go tool wails3 generate appimage \
    -binary "$APP_BINARY" \
    -icon "$STAGING_ICON" \
    -desktopfile "$DESKTOP_FILE" \
    -outputdir "$APPIMAGE_OUTPUT" \
    -builddir "$APPIMAGE_BUILD"

BASE_APPIMAGE="$APPIMAGE_OUTPUT/onecatch-$APPIMAGE_ARCH.AppImage"
if [ ! -f "$BASE_APPIMAGE" ]; then
    echo "error: Wails did not create the expected AppImage: $BASE_APPIMAGE" >&2
    exit 1
fi

# Wails bundles the main GUI and GTK runtime. Extract that image, add the three
# helper executables that OneCatch resolves next to itself, then repack it.
EXTRACT_ROOT="$STAGING_ROOT/extract"
mkdir -p "$EXTRACT_ROOT"
(
    cd "$EXTRACT_ROOT"
    "$BASE_APPIMAGE" --appimage-extract >/dev/null
)
APP_DIR="$EXTRACT_ROOT/squashfs-root"
for helper in "$WORKER_BINARY" "$SHELL_BINARY" "$ASKPASS_BINARY"; do
    install -m 0755 "$helper" "$APP_DIR/usr/bin/$(basename -- "$helper")"
done

APPIMAGE_TOOL="$STAGING_ROOT/appimagetool-$APPIMAGE_ARCH.AppImage"
curl --fail --location --retry 3 \
    --output "$APPIMAGE_TOOL" \
    "https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-$APPIMAGE_ARCH.AppImage"
chmod 0755 "$APPIMAGE_TOOL"
REPACKED_APPIMAGE="$STAGING_ROOT/OneCatch.AppImage"
ARCH="$APPIMAGE_ARCH" "$APPIMAGE_TOOL" --appimage-extract-and-run "$APP_DIR" "$REPACKED_APPIMAGE"
mv -f "$REPACKED_APPIMAGE" "$OUTPUT_APPIMAGE"

DEB_NAME=$(basename -- "$OUTPUT_DEB" .deb)
VERSION="$VERSION" GOARCH="$GO_ARCH" \
    go tool wails3 tool package \
        -name "$DEB_NAME" \
        -format deb \
        -config "$NFPM_CONFIG" \
        -out "$(dirname -- "$OUTPUT_DEB")"

write_checksum() {
    artifact=$1
    (
        cd "$(dirname -- "$artifact")"
        sha256sum "$(basename -- "$artifact")"
    ) > "$artifact.sha256"
}
write_checksum "$OUTPUT_APPIMAGE"
write_checksum "$OUTPUT_DEB"

echo "AppImage: $OUTPUT_APPIMAGE"
echo "Debian package: $OUTPUT_DEB"
echo "Version: $VERSION"
echo "Architecture: $GO_ARCH"
echo "Checksums: $OUTPUT_APPIMAGE.sha256, $OUTPUT_DEB.sha256"
