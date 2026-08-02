#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
APP_NAME=Oneshot
EXECUTABLE_NAME=oneshot
BINARY="$REPO_ROOT/bin/$EXECUTABLE_NAME"
WORKER_BINARY="$REPO_ROOT/bin/oneshot-worker"
INFO_PLIST="$SCRIPT_DIR/Info.plist"
ASSETS_CAR="$SCRIPT_DIR/Assets.car"
ICON_FILE="$SCRIPT_DIR/icons.icns"
PLIST_BUDDY=/usr/libexec/PlistBuddy

for tool in codesign ditto lipo plutil shasum; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "error: required command not found: $tool" >&2
        exit 1
    fi
done
if [ ! -x "$PLIST_BUDDY" ]; then
    echo "error: required command not found: $PLIST_BUDDY" >&2
    exit 1
fi

for input in "$BINARY" "$WORKER_BINARY" "$INFO_PLIST" "$ASSETS_CAR" "$ICON_FILE"; do
    if [ ! -f "$input" ]; then
        echo "error: required build input not found: $input" >&2
        exit 1
    fi
done
if [ ! -x "$BINARY" ]; then
    echo "error: built binary is not executable: $BINARY" >&2
    exit 1
fi
if [ ! -x "$WORKER_BINARY" ]; then
    echo "error: built worker is not executable: $WORKER_BINARY" >&2
    exit 1
fi

VERSION=${VERSION:-$($PLIST_BUDDY -c 'Print :CFBundleShortVersionString' "$INFO_PLIST")}
BUILD_ID=${BUILD_ID:-$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || printf 'dev')}
ARCHITECTURES=$(lipo -archs "$BINARY")
ARCH_LABEL=$(printf '%s' "$ARCHITECTURES" | tr ' ' '-')
OUTPUT_ZIP=${OUTPUT_ZIP:-$REPO_ROOT/bin/$APP_NAME-$VERSION-$BUILD_ID-$ARCH_LABEL.zip}
SIGN_IDENTITY=${SIGN_IDENTITY:--}

case "$OUTPUT_ZIP" in
    /*) ;;
    *) OUTPUT_ZIP="$REPO_ROOT/$OUTPUT_ZIP" ;;
esac

STAGING_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/oneshot-package.XXXXXX")
cleanup() {
    rm -rf -- "${STAGING_ROOT:?}"
}
trap cleanup EXIT HUP INT TERM

APP_BUNDLE="$STAGING_ROOT/$APP_NAME.app"
CONTENTS="$APP_BUNDLE/Contents"
mkdir -p "$CONTENTS/MacOS" "$CONTENTS/Resources/bin" "$(dirname -- "$OUTPUT_ZIP")"

install -m 0755 "$BINARY" "$CONTENTS/MacOS/$EXECUTABLE_NAME"
install -m 0755 "$WORKER_BINARY" "$CONTENTS/Resources/bin/oneshot-worker"
install -m 0644 "$INFO_PLIST" "$CONTENTS/Info.plist"
install -m 0644 "$ASSETS_CAR" "$CONTENTS/Resources/Assets.car"
install -m 0644 "$ICON_FILE" "$CONTENTS/Resources/icons.icns"
$PLIST_BUDDY -c "Set :CFBundleVersion $VERSION" "$CONTENTS/Info.plist"
$PLIST_BUDDY -c "Set :CFBundleShortVersionString $VERSION" "$CONTENTS/Info.plist"
plutil -lint "$CONTENTS/Info.plist" >/dev/null

if command -v xattr >/dev/null 2>&1; then
    xattr -cr "$APP_BUNDLE"
fi

if [ "$SIGN_IDENTITY" = "-" ]; then
    codesign --force --deep --sign - "$APP_BUNDLE"
else
    codesign --force --deep --options runtime --timestamp --sign "$SIGN_IDENTITY" "$APP_BUNDLE"
fi
codesign --verify --deep --strict --verbose=2 "$APP_BUNDLE"

ARCHIVE_TMP="$STAGING_ROOT/$APP_NAME.zip"
ditto -c -k --sequesterRsrc --keepParent "$APP_BUNDLE" "$ARCHIVE_TMP"
mv -f "$ARCHIVE_TMP" "$OUTPUT_ZIP"

echo "Package: $OUTPUT_ZIP"
echo "Version: $VERSION"
echo "Architectures: $ARCHITECTURES"
echo "Signing identity: $SIGN_IDENTITY"
shasum -a 256 "$OUTPUT_ZIP"
