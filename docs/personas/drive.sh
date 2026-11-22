#!/usr/bin/env bash
# Runs the dashboard on the fixture state, in a terminal that a persona can type into and read, in tmux (or in cmux when there is no tmux).
#
#   drive.sh start      builds the fixture and starts `sitrep watch` in it, 110 columns by 34 rows
#   drive.sh env        prints the exports that point `sitrep` at the fixture, for the persona's own shell commands
#   drive.sh screen     prints what the dashboard shows now
#   drive.sh keys K...  presses keys: single characters (j, ?, /) and names (enter esc tab shift+tab up down left right
#                       home end pgup pgdn space backspace ctrl+c ctrl+d ctrl+u); one argument is one key press
#   drive.sh type TEXT  types the text as it is (for a filter or a name), without enter
#   drive.sh stop       ends the run and deletes the fixture
#
# It uses the real `sitrep` on PATH (SITREP_BIN overrides it), only the fixture's state, and never anything under ~/.claude. tmux runs on
# a server of its own (socket sitrep-persona), so the user's tmux is not touched. DRIVE_BACKEND=cmux or tmux forces a backend.
set -euo pipefail
export CMUX_QUIET=1

here=$(cd "$(dirname "$0")" && pwd)
cur=${TMPDIR:-/tmp}/sitrep-persona.cur
bin=${SITREP_BIN:-sitrep}
sock=sitrep-persona

tm() { env -u TMUX tmux -L "$sock" "$@"; }

load() {
	[ -f "$cur" ] || { echo "drive.sh: nothing started; run 'drive.sh start' first" >&2; exit 1; }
	# shellcheck disable=SC1090
	. "$cur"
}

# tmux names its keys differently from ours.
tmux_key() {
	case $1 in
	enter) echo Enter ;; esc) echo Escape ;; tab) echo Tab ;; shift+tab) echo BTab ;;
	up) echo Up ;; down) echo Down ;; left) echo Left ;; right) echo Right ;;
	home) echo Home ;; end) echo End ;; pgup) echo PgUp ;; pgdn) echo PgDn ;;
	space) echo Space ;; backspace) echo BSpace ;; ctrl+[a-z]) echo "C-${1#ctrl+}" ;;
	*) echo "drive.sh: unknown key '$1'" >&2; return 1 ;;
	esac
}

case ${1:-} in
start)
	[ ! -f "$cur" ] || { echo "drive.sh: a run is already open; run 'drive.sh stop' first" >&2; exit 1; }
	backend=${DRIVE_BACKEND:-}
	if [ -z "$backend" ]; then
		if command -v tmux >/dev/null; then backend=tmux; elif command -v cmux >/dev/null; then backend=cmux; fi
	fi
	[ -n "$backend" ] || { echo "drive.sh: tmux or cmux is needed" >&2; exit 1; }
	root=$(mktemp -d "${TMPDIR:-/tmp}/sitrep-persona.XXXXXX")
	root=$(cd "$root" && pwd -P)
	"$here/fixture.sh" "$root" >/dev/null
	run="env SITREP_STATE_DIR='$root/state' CLAUDE_CONFIG_DIR='$root/claude' '$bin' watch"
	ws="" surface=""
	if [ "$backend" = tmux ]; then
		tm new-session -d -s persona -x 110 -y 34 "$run"
	else
		out=$(cmux workspace create --name sitrep-persona --focus false --command "$run")
		ws=$(printf '%s\n' "$out" | grep -o 'workspace:[0-9A-Za-z-]*' | head -1)
		sleep 2
		surface=$(cmux list-pane-surfaces --workspace "$ws" | grep -o 'surface:[0-9A-Za-z-]*' | head -1)
	fi
	printf 'backend=%q\nroot=%q\nws=%q\nsurface=%q\n' "$backend" "$root" "$ws" "$surface" >"$cur"
	sleep 1
	echo "started with $backend (state $root/state)"
	;;
env)
	load
	echo "export SITREP_STATE_DIR='$root/state' CLAUDE_CONFIG_DIR='$root/claude'"
	;;
screen)
	load
	if [ "$backend" = tmux ]; then tm capture-pane -p -t persona; else cmux read-screen --surface "$surface" --lines 60; fi
	;;
keys)
	load
	shift
	for k in "$@"; do
		if [ "$backend" = tmux ]; then
			if [ "${#k}" -eq 1 ]; then tm send-keys -t persona -l -- "$k"; else tm send-keys -t persona "$(tmux_key "$k")"; fi
		elif [ "${#k}" -eq 1 ]; then
			cmux send --surface "$surface" -- "$k" >/dev/null
		else
			cmux send-key --surface "$surface" "$k" >/dev/null
		fi
		sleep 0.3
	done
	;;
type)
	load
	text=${2?usage: drive.sh type TEXT}
	if [ "$backend" = tmux ]; then tm send-keys -t persona -l -- "$text"; else cmux send --surface "$surface" -- "$text" >/dev/null; fi
	sleep 0.3
	;;
stop)
	[ -f "$cur" ] || exit 0
	load
	if [ "$backend" = tmux ]; then tm kill-server >/dev/null 2>&1 || true; else cmux workspace close "$ws" --force >/dev/null 2>&1 || true; fi
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
