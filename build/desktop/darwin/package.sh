#!/bin/sh

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)
APP_NAME=OneCatch
EXECUTABLE_NAME=onecatch
BINARY="$REPO_ROOT/bin/$EXECUTABLE_NAME"
WORKER_BINARY="$REPO_ROOT/bin/onecatch-worker"
SHELL_BINARY="$REPO_ROOT/bin/onecatchsh"
ASKPASS_BINARY="$REPO_ROOT/bin/onecatch-askpass"
UPDATER_BINARY="$REPO_ROOT/bin/onecatch-updater"
INFO_PLIST="$SCRIPT_DIR/Info.plist"
ASSETS_CAR="$SCRIPT_DIR/Assets.car"
ICON_FILE="$SCRIPT_DIR/icons.icns"
VERSION_FILE="$REPO_ROOT/VERSION"
PLIST_BUDDY=/usr/libexec/PlistBuddy

for tool in codesign ditto hdiutil lipo plutil shasum; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "error: required command not found: $tool" >&2
        exit 1
    fi
done
if [ ! -x "$PLIST_BUDDY" ]; then
    echo "error: required command not found: $PLIST_BUDDY" >&2
    exit 1
fi

for input in "$BINARY" "$WORKER_BINARY" "$SHELL_BINARY" "$ASKPASS_BINARY" "$UPDATER_BINARY" "$INFO_PLIST" "$ASSETS_CAR" "$ICON_FILE" "$VERSION_FILE"; do
    if [ ! -f "$input" ]; then
        echo "error: required build input not found: $input" >&2
        exit 1
    fi
done
for executable in "$BINARY" "$WORKER_BINARY" "$SHELL_BINARY" "$ASKPASS_BINARY" "$UPDATER_BINARY"; do
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

ARCHITECTURES=$(lipo -archs "$BINARY")
ARCH_LABEL=$(printf '%s' "$ARCHITECTURES" | tr ' ' '-')
OUTPUT_DMG=${OUTPUT_DMG:-$REPO_ROOT/bin/$APP_NAME-$VERSION-macOS-$ARCH_LABEL.dmg}
GO_ARCH=$(go env GOARCH)
OUTPUT_UPDATE_ZIP=${OUTPUT_UPDATE_ZIP:-$REPO_ROOT/bin/$APP_NAME-$VERSION-darwin-$GO_ARCH.zip}
SIGN_IDENTITY=${SIGN_IDENTITY:--}
NOTARY_PROFILE=${NOTARY_PROFILE:-}

case "$OUTPUT_DMG" in
    /*) ;;
    *) OUTPUT_DMG="$REPO_ROOT/$OUTPUT_DMG" ;;
esac
case "$OUTPUT_UPDATE_ZIP" in
    /*) ;;
    *) OUTPUT_UPDATE_ZIP="$REPO_ROOT/$OUTPUT_UPDATE_ZIP" ;;
esac

STAGING_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/onecatch-package.XXXXXX")
cleanup() {
    rm -rf -- "${STAGING_ROOT:?}"
}
trap cleanup EXIT HUP INT TERM

DMG_ROOT="$STAGING_ROOT/dmg"
APP_BUNDLE="$DMG_ROOT/$APP_NAME.app"
CONTENTS="$APP_BUNDLE/Contents"
mkdir -p "$CONTENTS/MacOS" "$CONTENTS/Resources/bin" "$(dirname -- "$OUTPUT_DMG")"

install -m 0755 "$BINARY" "$CONTENTS/MacOS/$EXECUTABLE_NAME"
install -m 0755 "$WORKER_BINARY" "$CONTENTS/Resources/bin/onecatch-worker"
install -m 0755 "$SHELL_BINARY" "$CONTENTS/Resources/bin/onecatchsh"
install -m 0755 "$ASKPASS_BINARY" "$CONTENTS/Resources/bin/onecatch-askpass"
install -m 0755 "$UPDATER_BINARY" "$CONTENTS/Resources/bin/onecatch-updater"
install -m 0644 "$INFO_PLIST" "$CONTENTS/Info.plist"
install -m 0644 "$ASSETS_CAR" "$CONTENTS/Resources/Assets.car"
install -m 0644 "$ICON_FILE" "$CONTENTS/Resources/icons.icns"
$PLIST_BUDDY -c "Set :CFBundleVersion $VERSION" "$CONTENTS/Info.plist"
$PLIST_BUDDY -c "Set :CFBundleShortVersionString $VERSION" "$CONTENTS/Info.plist"
plutil -lint "$CONTENTS/Info.plist" >/dev/null

if command -v xattr >/dev/null 2>&1; then
    xattr -cr "$APP_BUNDLE"
fi

sign_path() {
    if [ "$SIGN_IDENTITY" = "-" ]; then
        codesign --force --sign - "$1"
    else
        codesign --force --options runtime --timestamp --sign "$SIGN_IDENTITY" "$1"
    fi
}

# Sign nested executables first so the outer bundle seal contains their final
# signatures. This also works with the default ad-hoc identity used for tests.
sign_path "$CONTENTS/Resources/bin/onecatch-worker"
sign_path "$CONTENTS/Resources/bin/onecatchsh"
sign_path "$CONTENTS/Resources/bin/onecatch-askpass"
sign_path "$CONTENTS/Resources/bin/onecatch-updater"
sign_path "$CONTENTS/MacOS/$EXECUTABLE_NAME"
sign_path "$APP_BUNDLE"
codesign --verify --deep --strict --verbose=2 "$APP_BUNDLE"

ln -s /Applications "$DMG_ROOT/Applications"
rm -f "$OUTPUT_DMG"
hdiutil create \
    -volname "$APP_NAME $VERSION" \
    -srcfolder "$DMG_ROOT" \
    -ov \
    -format UDZO \
    "$OUTPUT_DMG"

if [ "$SIGN_IDENTITY" != "-" ]; then
    codesign --force --timestamp --sign "$SIGN_IDENTITY" "$OUTPUT_DMG"
    codesign --verify --verbose=2 "$OUTPUT_DMG"
fi

if [ -n "$NOTARY_PROFILE" ]; then
    if ! command -v xcrun >/dev/null 2>&1; then
        echo "error: NOTARY_PROFILE requires xcrun" >&2
        exit 1
    fi
    xcrun notarytool submit "$OUTPUT_DMG" --keychain-profile "$NOTARY_PROFILE" --wait
    # The update ZIP contains the app rather than the DMG, so staple the same
    # notarisation ticket onto the app before archiving it.
    xcrun stapler staple "$APP_BUNDLE"
    xcrun stapler validate "$APP_BUNDLE"
    xcrun stapler staple "$OUTPUT_DMG"
    xcrun stapler validate "$OUTPUT_DMG"
fi

rm -f "$OUTPUT_UPDATE_ZIP"
# The updater's extractor intentionally accepts exactly one top-level entry.
# --sequesterRsrc would add a sibling __MACOSX directory and make an otherwise
# valid archive fail that invariant.
ditto --norsrc --noextattr --noqtn --noacl -c -k --keepParent "$APP_BUNDLE" "$OUTPUT_UPDATE_ZIP"

CHECKSUM_FILE="$OUTPUT_DMG.sha256"
(
    cd "$(dirname -- "$OUTPUT_DMG")"
    shasum -a 256 "$(basename -- "$OUTPUT_DMG")"
) > "$CHECKSUM_FILE"
UPDATE_CHECKSUM_FILE="$OUTPUT_UPDATE_ZIP.sha256"
(
    cd "$(dirname -- "$OUTPUT_UPDATE_ZIP")"
    shasum -a 256 "$(basename -- "$OUTPUT_UPDATE_ZIP")"
) > "$UPDATE_CHECKSUM_FILE"

echo "Installer: $OUTPUT_DMG"
echo "Updater archive: $OUTPUT_UPDATE_ZIP"
echo "Version: $VERSION"
echo "Architectures: $ARCHITECTURES"
echo "Signing identity: $SIGN_IDENTITY"
echo "Checksum: $CHECKSUM_FILE"
cat "$CHECKSUM_FILE"
cat "$UPDATE_CHECKSUM_FILE"
