#!/bin/sh

set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

if [ "$(uname -s)" = "Darwin" ]; then
	exec "$script_dir/darwin/run-dev-bundle.sh"
fi

project_root="$(CDPATH= cd -- "$script_dir/../.." && pwd)"
binary="$project_root/bin/onecatch"

if [ ! -x "$binary" ]; then
	echo "development binary not found: $binary" >&2
	exit 1
fi

cd "$project_root"
exec "$binary"
