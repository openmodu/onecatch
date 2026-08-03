#!/bin/sh

set -u

mode="${1:-}"
watcher_pid="${2:-}"

if [ "$#" -lt 3 ] || { [ "$mode" != "background" ] && [ "$mode" != "primary" ]; }; then
	echo "usage: $0 <background|primary> <watcher-pid> <command> [args...]" >&2
	exit 2
fi

shift 2

process_group="$(ps -o pgid= -p "$$" | tr -d '[:space:]')"
child_pid=""
monitor_pid=""

stop_local_processes() {
	trap - HUP INT TERM

	if [ -n "$monitor_pid" ]; then
		kill -TERM "$monitor_pid" 2>/dev/null || true
	fi
	if [ -n "$child_pid" ]; then
		kill -TERM "$child_pid" 2>/dev/null || true
	fi

	exit 0
}

relay_to_watcher() {
	signal="$1"
	trap - HUP INT TERM

	# The supervised app receives the terminal signal with this process group.
	# Relay the same signal to Wails so it can stop every configured process.
	kill "-$signal" "$watcher_pid" 2>/dev/null || true
	exit 0
}

if [ "$mode" = "primary" ]; then
	trap 'relay_to_watcher HUP' HUP
	trap 'relay_to_watcher INT' INT
	trap 'relay_to_watcher TERM' TERM
else
	trap stop_local_processes HUP INT TERM
fi

"$@" &
child_pid="$!"

# Refresh places every async command in its own process group. If the Wails
# watcher disappears without running its cleanup, terminate that whole group so
# descendants such as npm, Vite and esbuild cannot become orphan processes.
(
	trap - HUP INT TERM
	while kill -0 "$watcher_pid" 2>/dev/null; do
		sleep 1
	done
	kill -TERM "-$process_group" 2>/dev/null || true
) &
monitor_pid="$!"

wait "$child_pid"
status="$?"

kill -TERM "$monitor_pid" 2>/dev/null || true
wait "$monitor_pid" 2>/dev/null || true

exit "$status"
