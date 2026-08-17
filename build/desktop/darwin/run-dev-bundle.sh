#!/bin/sh

set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
project_root="$(CDPATH= cd -- "$script_dir/../../.." && pwd)"
binary="$project_root/bin/onecatch"
app_bundle="$project_root/bin/OneCatch.dev.app"
contents="$app_bundle/Contents"
macos_dir="$contents/MacOS"
resources_dir="$contents/Resources"

if [ ! -x "$binary" ]; then
	echo "development binary not found: $binary" >&2
	exit 1
fi

mkdir -p "$macos_dir" "$resources_dir"
cp "$binary" "$macos_dir/onecatch"
chmod +x "$macos_dir/onecatch"
cp "$script_dir/Info.dev.plist" "$contents/Info.plist"
cp "$script_dir/icons.icns" "$resources_dir/icons.icns"

if [ -f "$script_dir/Assets.car" ]; then
	cp "$script_dir/Assets.car" "$resources_dir/Assets.car"
fi

# A real application bundle gives macOS the LaunchServices identity used for
# the icon badge on a minimized window thumbnail. Ad-hoc signing keeps the
# refreshed development bundle internally consistent after each rebuild.
codesign --force --deep --sign - "$app_bundle" >/dev/null

cd "$project_root"
exec "$macos_dir/onecatch"
