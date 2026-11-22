#!/usr/bin/env bash
# Drives the demo in DEMO.md through cmux: it types the cues and commands into the demo
# terminals and waits on sitrep's own state, never on what the screen shows.
#
#   DEMO_API=surface:2 DEMO_WEB=surface:3 DEMO_SHELL=surface:4 ./demo.sh check
#   ./demo.sh reset | cue | cue-api | cue-web | scene1 .. scene6 | all
#   scene1 starts the dashboard and then runs cue-api and cue-web, so they happen on screen.
#
# The three surfaces are required and are never defaulted: cmux would otherwise type into
# the terminal this script runs in. Start `claude` in the api and web terminals, and export
# SITREP_STATE_DIR in all three, before anything here runs. `cmux send` refuses a terminal
# with a draft or an open dialog, and that stops the script instead of typing over it.
set -euo pipefail

: "${DEMO_API:?set DEMO_API to the cmux surface of the api session}"
: "${DEMO_WEB:?set DEMO_WEB to the cmux surface of the web session}"
: "${DEMO_SHELL:?set DEMO_SHELL to the cmux surface of the plain shell}"
export SITREP_STATE_DIR="${SITREP_STATE_DIR:-$HOME/demo-state}"
WAIT="${DEMO_WAIT:-180}"

# shellcheck disable=SC2016 # the backticks are literal
API_CUE='In `app.py`, add `limit` and `offset` to `list_orders`. Then write unittest tests for them in `test_orders.py` next to it and run them with `python3 -m unittest test_orders`. Keep it small. Do not ask, just do it.'
WEB_CUE='Before changing anything, ask me in plain text, without using a question tool, whether dark mode should follow the system setting or a toggle. Then stop and wait for my answer.'

say() { printf '\n== %s\n' "$*"; }
# With DEMO_AUTO the script waits DEMO_GAP seconds where it would wait for you, so a recording has time to breathe.
pause() {
	if [ -n "${DEMO_AUTO:-}" ]; then sleep "${DEMO_GAP:-4}"; else read -r -p "-- $* (enter to go on) " _; fi
}

# at_prompt: the recorded shell's last line is a bare prompt, with no draft and no program
# (such as `sitrep dash`) that would take the typing as keys. DEMO_PROMPT_RE matches that line.
at_prompt() {
	local re=${DEMO_PROMPT_RE:-}
	[ -n "$re" ] || re='(^|[[:space:]])\$$'
	cmux read-screen --surface "$DEMO_SHELL" --lines 5 | sed 's/[[:space:]]*$//' | grep -v '^$' | tail -1 | grep -Eq "$re"
}

# require_prompt waits a few seconds for the previous command to finish, then gives up.
require_prompt() {
	local end=$((SECONDS + ${DEMO_PROMPT_WAIT:-10}))
	until at_prompt; do
		if [ "$SECONDS" -ge "$end" ]; then
			echo "the shell $DEMO_SHELL is not at a bare prompt: quit the program in it or clear its draft" >&2
			return 1
		fi
		sleep 1
	done
}

# clear_draft SURFACE: a blocked prompt stays in a Claude input box, and the next text would be
# added to it. Each ctrl+u removes one wrapped line, so it repeats until the prompt line is empty.
clear_draft() {
	local line
	for _ in 1 2 3 4 5; do
		line=$(cmux read-screen --surface "$1" --lines 14 | grep '^❯' | tail -1 | sed 's/^❯[[:space:]]*//')
		[ -n "$line" ] || return 0
		cmux send-key --surface "$1" ctrl+u >/dev/null
		sleep 1
	done
	echo "could not clear the input box of $1" >&2
	return 1
}

# type_into SURFACE TEXT: types the text and presses enter. The shell is typed into only at a prompt.
type_into() {
	[ "$1" != "$DEMO_SHELL" ] || require_prompt
	cmux send --surface "$1" -- "$2"
	cmux send-key --surface "$1" enter
	# Lets the screen redraw before the next prompt check, and gives a viewer time to read.
	[ "$1" != "$DEMO_SHELL" ] || sleep "${DEMO_BEAT:-3}"
}

snapshot() { sitrep ls --json; }

# field DIR JQ-FIELD: a field of the session whose directory ends in DIR.
field() {
	snapshot | jq -r --arg d "$1" '[.sessions[] | select(.dir == $d)][0] | '"$2"
}

# wait_for DESCRIPTION JQ-TEST: polls the snapshot until the test is true.
wait_for() {
	local what=$1 test=$2 end=$((SECONDS + WAIT))
	while ! snapshot | jq -e "$test" >/dev/null 2>&1; do
		if [ "$SECONDS" -ge "$end" ]; then
			echo "timed out after ${WAIT}s waiting for: $what" >&2
			return 1
		fi
		sleep 2
	done
}

session_test() { # DIR JQ-CONDITION
	printf '[.sessions[] | select(.dir == "%s")][0] | %s' "$1" "$2"
}

check() {
	say "checking"
	command -v jq >/dev/null || { echo "jq is not installed" >&2; return 1; }
	local v
	v=$(sitrep version)
	echo "sitrep $v, state in $SITREP_STATE_DIR"
	for s in "$DEMO_API" "$DEMO_WEB" "$DEMO_SHELL"; do
		cmux read-screen --surface "$s" --lines 3 >/dev/null
	done
	# A session is listed only after its first prompt, so cue is what makes them appear.
	echo "ok: the three surfaces answer"
}

# reset puts the two demo projects back to their committed files, so a take starts with work
# to do. It restores tracked files, and removes the one file the api cue makes, by its own path.
reset() {
	say "reset the demo projects"
	local root=$HOME/demo-projects d
	for d in shop-api shop-web; do
		git -C "$root/$d" rev-parse --is-inside-work-tree >/dev/null
		git -C "$root/$d" checkout -- .
	done
	rm -f ~/demo-projects/shop-api/test_orders.py
	for d in shop-api shop-web; do git -C "$root/$d" status --short; done
	reset_state
	echo "ok"
}

# reset_state frees the names api and web, which an earlier take's ended sessions still hold
# and sitrep refuses to give twice, and empties the demo inbox.
reset_state() {
	local name id
	for name in api web; do
		id=$(sitrep alias | awk -v n="$name" '$1 == n { print $2 }')
		[ -z "$id" ] || sitrep alias "$id" --off >/dev/null
	done
	for _ in $(seq 1 20); do
		[ "$(snapshot | jq '.inbox.count')" -gt 0 ] || break
		sitrep rm inbox 1 >/dev/null
	done
}

# cue_api and cue_web are separate so a recording can start the dashboard first and let the
# audience watch each session change. cue does both, for when nothing is on screen yet.
cue_api() {
	say "cue: api works"
	type_into "$DEMO_API" "$API_CUE"
	# A session is listed only after its first prompt.
	wait_for "api to be listed" "$(session_test shop-api '.state != null')"
}

cue_web() {
	say "cue: web stops to ask"
	type_into "$DEMO_WEB" "$WEB_CUE"
	wait_for "web to stop and wait for the answer" "$(session_test shop-web '.needs_input')"
	echo "web is waiting"
}

cue() {
	cue_api
	cue_web
}

# quit_dashboard presses q when a program (the dashboard) holds the shell; the next command
# typed into the shell waits for the prompt to come back.
quit_dashboard() {
	at_prompt || cmux send-key --surface "$DEMO_SHELL" q >/dev/null
}

scene1() {
	say "1. the glance"
	type_into "$DEMO_SHELL" 'sitrep dash'
	sleep 2
	cue_api
	cue_web
	echo "Press enter on the web row to show the jump, then come back."
	pause "back from the jump"
}

scene2() {
	say "2. capture"
	type_into "$DEMO_API" '+inbox look at the slow dashboard query later'
	wait_for "the inbox to hold 1 item" '.inbox.count >= 1'
	clear_draft "$DEMO_API"
}

scene3() {
	say "3. triage"
	local api web
	quit_dashboard
	for t in 'fix the flaky checkout test' 'add pagination to the orders endpoint' 'check the contrast in dark mode'; do
		type_into "$DEMO_SHELL" "sitrep add inbox '$t'"
		sleep 1
	done
	type_into "$DEMO_SHELL" 'sitrep ls'
	pause "show the ids"
	api=$(field shop-api .short)
	web=$(field shop-web .short)
	type_into "$DEMO_SHELL" "sitrep alias $api api"
	type_into "$DEMO_SHELL" "sitrep alias $web web"
	# The inbox is [slow query, flaky test, pagination, contrast]; --item guards each position.
	type_into "$DEMO_SHELL" "sitrep move inbox api 2 --item 'fix the flaky checkout test'"
	type_into "$DEMO_SHELL" "sitrep move inbox api 2 --item 'add pagination to the orders endpoint'"
	type_into "$DEMO_SHELL" "sitrep move inbox web 2 --item 'check the contrast in dark mode'"
	type_into "$DEMO_SHELL" 'sitrep ls -v'
}

scene4() {
	say "4. work and finish"
	type_into "$DEMO_API" 'fix the flaky checkout test'
	wait_for "api to start" "$(session_test shop-api '.state == "working"')"
	wait_for "api to finish its turn" "$(session_test shop-api '.state == "waiting"')"
	type_into "$DEMO_SHELL" 'sitrep done api'
}

scene5() {
	say "5. fix a wrong turn"
	type_into "$DEMO_SHELL" "sitrep move api web 1 --item 'add pagination to the orders endpoint'"
}

scene6() {
	say "6. it is just commands"
	type_into "$DEMO_SHELL" "sitrep add web 'update the settings page copy'"
	type_into "$DEMO_SHELL" "sitrep ls --json | jq '.sessions[] | {id, alias, state, queue: .queue.count}'"
	sleep 2
	type_into "$DEMO_SHELL" 'sitrep dash'
}

case "${1:-}" in
check) check ;;
reset) reset ;;
cue) cue ;;
cue-api) cue_api ;;
cue-web) cue_web ;;
scene[1-6]) "$1" ;;
all)
	check
	reset
	for s in scene1 scene2 scene3 scene4 scene5 scene6; do
		pause "next: $s"
		"$s"
	done
	pause "end"
	;;
*) echo "usage: demo.sh check | reset | cue | cue-api | cue-web | scene1..scene6 | all" >&2; exit 2 ;;
esac
