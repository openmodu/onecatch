#!/bin/sh
set -eu

# Keep Xcode Run and task builds on the same lifecycle and release metadata.
app_version=$(node build/scripts/release-info.mjs --version)
plist=build/ios/xcode/main/Info.plist
cp build/ios/Info.plist "$plist"
/usr/libexec/PlistBuddy -c 'Set :CFBundleExecutable $(EXECUTABLE_NAME)' "$plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $app_version" "$plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $app_version" "$plist"
