#!/usr/bin/env bash
# Drives the demo in DEMO.md through cmux: it types the cues and commands into the demo
# terminals and waits on sitrep's own state, never on what the screen shows.
#
#   DEMO_API=surface:2 DEMO_WEB=surface:3 DEMO_SHELL=surface:4 ./demo.sh check
#   ./demo.sh cue | scene1 .. scene6 | all
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
API_CUE='In `app.py`, add `limit` and `offset` to `list_orders`. Keep the change small. Do not ask, just make the change.'
WEB_CUE='Before changing anything, ask me in plain text, without using a question tool, whether dark mode should follow the system setting or a toggle. Then stop and wait for my answer.'

say() { printf '\n== %s\n' "$*"; }
pause() { [ -n "${DEMO_AUTO:-}" ] || read -r -p "-- $* (enter to go on) " _; }

# type SURFACE TEXT: types the text and presses enter.
type_into() {
	cmux send --surface "$1" -- "$2"
	cmux send-key --surface "$1" enter
}

snapshot() { sitrep ls --json; }

# field DIR JQ-FIELD: a field of the session whose directory ends in DIR.
field() {
	snapshot | jq -r --arg d "$1" '[.sessions[] | select(.dir | endswith("/" + $d))][0] | '"$2"
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
	printf '[.sessions[] | select(.dir | endswith("/%s"))][0] | %s' "$1" "$2"
}

check() {
	say "checking"
	command -v jq >/dev/null || { echo "jq is not installed" >&2; return 1; }
	local v
	v=$(sitrep version)
	echo "sitrep $v, state in $SITREP_STATE_DIR"
	for d in shop-api shop-web; do
		[ "$(field "$d" '.id // empty')" ] || { echo "no session for $d yet: start claude there" >&2; return 1; }
	done
	for s in "$DEMO_API" "$DEMO_WEB" "$DEMO_SHELL"; do
		cmux read-screen --surface "$s" --lines 3 >/dev/null
	done
	echo "ok: both sessions are known and the three surfaces answer"
}

cue() {
	say "cue: api works, web stops to ask"
	type_into "$DEMO_API" "$API_CUE"
	type_into "$DEMO_WEB" "$WEB_CUE"
	wait_for "web to stop and wait for the answer" "$(session_test shop-web '.needs_input')"
	echo "web is waiting"
}

scene1() {
	say "1. the glance"
	type_into "$DEMO_SHELL" 'sitrep watch'
	echo "Press enter on the web row to show the jump, then come back and press q."
	pause "back from the jump"
}

scene2() {
	say "2. capture"
	type_into "$DEMO_API" '+inbox look at the slow dashboard query later'
	wait_for "the inbox to hold 1 item" '.inbox.count >= 1'
}

scene3() {
	say "3. triage"
	local api web
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
	type_into "$DEMO_SHELL" 'sitrep watch'
}

case "${1:-}" in
check) check ;;
cue) cue ;;
scene[1-6]) "$1" ;;
all)
	check
	cue
	for s in scene1 scene2 scene3 scene4 scene5 scene6; do
		pause "next: $s"
		"$s"
	done
	;;
*) echo "usage: demo.sh check | cue | scene1..scene6 | all" >&2; exit 2 ;;
esac
