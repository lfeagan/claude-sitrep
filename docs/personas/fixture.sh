#!/usr/bin/env bash
# Builds a throwaway sitrep state folder for a persona run: a few sessions in different states, queues and an inbox.
#
#   fixture.sh <root>     makes <root>/state (SITREP_STATE_DIR) and <root>/claude (CLAUDE_CONFIG_DIR, empty)
#
# The two are siblings on purpose: sitrep refuses a state folder that holds the Claude folder. It writes the files
# of docs/STATE-LAYOUT.md directly, so it needs no Claude session and touches nothing under ~/.claude.
set -euo pipefail

root=${1:?usage: fixture.sh <root>}
case $root in /*) ;; *) echo "fixture.sh: <root> must be an absolute path" >&2; exit 2 ;; esac
[ ! -e "$root/state" ] || { echo "fixture.sh: $root/state exists" >&2; exit 1; }
umask 077
mkdir -p "$root/state/sessions" "$root/claude"
now=$(date +%s)

# session ID DIR STATE AGE_SECONDS OBJECTIVE PROGRESS NOW [ITEM...]
session() {
	local id=$1 dir=$2 st=$3 age=$4 obj=$5 prog=$6 nw=$7
	shift 7
	local d="$root/state/sessions/$id"
	mkdir -p "$d"
	printf 'Objective: %s\nProgress:  %s\nNow:       %s\n' "$obj" "$prog" "$nw" >"$d/notes"
	printf '/work/%s\n' "$dir" >"$d/cwd"
	printf '%s\n' "$st" >"$d/state"
	printf '1\n' >"$d/count"
	: >"$d/queue"
	for item in "$@"; do printf '%s\n' "$item" >>"$d/queue"; done
	perl -e 'utime $ARGV[0], $ARGV[0], @ARGV[1..$#ARGV]' $((now - age)) "$d/state" "$d/notes"
}

session 2f3a9c01 api waiting 180 \
	"Add pagination to list_orders" "limit and offset done, tests written" "Waiting for you: should offset be zero-based?" \
	"write the migration note" "bump the client version"
session 5b7d11e2 web working 20 \
	"Dark mode for the settings page" "toggle added" "Editing theme.css" \
	"check contrast on the footer"
session 91c04aa7 infra waiting 2400 \
	"Rotate the staging certificates" "new certs issued" "Waiting for you: restart the proxy?"
session c3e8f6b5 docs ended 7200 \
	"Rewrite the install guide" "published" "-"

printf '%s\n' "look at the nightly job" "reply to the review on the retry change" "rename the demo repo" >"$root/state/inbox.queue"
printf 'api\n' >"$root/state/sessions/2f3a9c01/alias"

echo "state:  $root/state"
echo "claude: $root/claude"
