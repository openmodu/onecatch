#!/bin/sh

set -u

if [ "$#" -lt 1 ]; then
	echo "usage: $0 <command> [args...]" >&2
	exit 2
fi

# Wails refresh runs its async commands in separate process groups. On macOS,
# one of the short-lived children can be left as the terminal's foreground
# group, so a later Ctrl+C is echoed without reaching any live process. Keep
# the Wails session inside its own pseudo-terminal and retain this wrapper in
# the user's terminal to receive and forward shutdown requests reliably.
if [ "$(uname -s)" != "Darwin" ]; then
	exec "$@"
fi

session_pid=""

stop_session() {
	trap - HUP INT TERM
	if [ -n "$session_pid" ]; then
		kill -TERM "$session_pid" 2>/dev/null || true
		wait "$session_pid" 2>/dev/null || true
	fi
	exit 0
}

trap stop_session HUP INT TERM

/usr/bin/script -q /dev/null "$@" &
session_pid="$!"

wait "$session_pid"
status="$?"
trap - HUP INT TERM

exit "$status"
