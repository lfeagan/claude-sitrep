#!/usr/bin/env bash
# Runs the dashboard in a throwaway cmux workspace, on the fixture state, so that a persona can press keys and read the screen.
#
#   drive.sh start      builds the fixture, opens a workspace running `sitrep watch` and prints how to reach it
#   drive.sh env        prints the exports that point `sitrep` at the fixture, for the persona's own shell commands
#   drive.sh screen     prints what the dashboard shows now
#   drive.sh keys K...  presses keys: single characters (j, ?, /) and names (enter esc tab shift+tab up down left right
#                       home end pgup pgdn space backspace ctrl+c ctrl+d ctrl+u); one argument is one key press
#   drive.sh type TEXT  types the text as it is (for a filter or a name), without enter
#   drive.sh stop       closes the workspace and deletes the fixture
#
# It uses the real `sitrep` on PATH (SITREP_BIN overrides it), only the fixture's state, and never anything under ~/.claude.
set -euo pipefail
export CMUX_QUIET=1

here=$(cd "$(dirname "$0")" && pwd)
cur=${TMPDIR:-/tmp}/sitrep-persona.cur
bin=${SITREP_BIN:-sitrep}

load() {
	[ -f "$cur" ] || { echo "drive.sh: nothing started; run 'drive.sh start' first" >&2; exit 1; }
	# shellcheck disable=SC1090
	. "$cur"
}

case ${1:-} in
start)
	[ ! -f "$cur" ] || { echo "drive.sh: a run is already open; run 'drive.sh stop' first" >&2; exit 1; }
	command -v cmux >/dev/null || { echo "drive.sh: cmux is needed" >&2; exit 1; }
	root=$(mktemp -d "${TMPDIR:-/tmp}/sitrep-persona.XXXXXX")
	root=$(cd "$root" && pwd -P)
	"$here/fixture.sh" "$root" >/dev/null
	out=$(cmux workspace create --name sitrep-persona --focus false \
		--command "env SITREP_STATE_DIR='$root/state' CLAUDE_CONFIG_DIR='$root/claude' '$bin' watch")
	ws=$(printf '%s\n' "$out" | grep -o 'workspace:[0-9A-Za-z-]*' | head -1)
	sleep 2
	surface=$(cmux list-pane-surfaces --workspace "$ws" | grep -o 'surface:[0-9A-Za-z-]*' | head -1)
	printf 'root=%q\nws=%q\nsurface=%q\n' "$root" "$ws" "$surface" >"$cur"
	echo "started: $ws $surface (state $root/state)"
	;;
env)
	load
	echo "export SITREP_STATE_DIR='$root/state' CLAUDE_CONFIG_DIR='$root/claude'"
	;;
screen)
	load
	cmux read-screen --surface "$surface" --lines 60
	;;
keys)
	load
	shift
	for k in "$@"; do
		if [ "${#k}" -eq 1 ]; then cmux send --surface "$surface" -- "$k" >/dev/null; else cmux send-key --surface "$surface" "$k" >/dev/null; fi
		sleep 0.3
	done
	;;
type)
	load
	cmux send --surface "$surface" -- "${2?usage: drive.sh type TEXT}" >/dev/null
	sleep 0.3
	;;
stop)
	[ -f "$cur" ] || exit 0
	load
	cmux workspace close "$ws" --force >/dev/null 2>&1 || true
	case $root in
	*/sitrep-persona.*) rm -r -- "$root" ;; # the fixture folder this script made
	esac
	rm -f -- "$cur"
	echo "stopped"
	;;
*)
	sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//' >&2
	exit 2
	;;
esac
