#!/usr/bin/env bash
# Builds a throwaway sitrep state folder for a persona run: a few sessions in different states, queues and an inbox.
#
#   fixture.sh <root> [--sessions N] [--hostile]
#
# It makes <root>/state (SITREP_STATE_DIR) and <root>/claude (CLAUDE_CONFIG_DIR, empty). --sessions N adds N more sessions in every state
# and age, with long and wide-character text. --hostile adds sessions whose files hold what a hostile transcript or a hand edit could put
# there: escape sequences, bidi overrides, C1 controls, huge lines, a huge queue, links and junk.
#
# The two are siblings on purpose: sitrep refuses a state folder that holds the Claude folder. It writes the files
# of docs/STATE-LAYOUT.md directly, so it needs no Claude session and touches nothing under ~/.claude.
set -euo pipefail

root=${1:?usage: fixture.sh <root> [--sessions N] [--hostile]}
shift
extra=0 hostile=""
while [ $# -gt 0 ]; do
	case $1 in
	--sessions) extra=${2:?--sessions needs a number}; shift 2 ;;
	--hostile) hostile=1; shift ;;
	*) echo "fixture.sh: unknown option $1" >&2; exit 2 ;;
	esac
done
case $extra in '' | *[!0-9]*) echo "fixture.sh: --sessions needs a number" >&2; exit 2 ;; esac
case $root in /*) ;; *) echo "fixture.sh: <root> must be an absolute path" >&2; exit 2 ;; esac
[ ! -e "$root/state" ] || { echo "fixture.sh: $root/state exists" >&2; exit 1; }
umask 077
mkdir -p "$root/state/sessions" "$root/claude"
now=$(date +%s)

# stamp EPOCH is the time as touch -t wants it: BSD date takes -r for seconds since the epoch, and GNU date takes -d @.
stamp() { date -r "$1" +%Y%m%d%H%M.%S 2>/dev/null || date -d "@$1" +%Y%m%d%H%M.%S; }

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
	touch -t "$(stamp $((now - age)))" "$d/state" "$d/notes"
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

# --sessions N: deterministic variety, so a run can be repeated
states=(waiting working waiting ended working waiting)
dirs=(api web infra docs mobile billing search auth ml-pipeline 日本語のプロジェクト)
for ((i = 1; i <= extra; i++)); do
	id=$(printf 's%03d-%03d' "$i" "$i")
	st=${states[$((i % ${#states[@]}))]}
	dir=${dirs[$((i % ${#dirs[@]}))]}-$i
	age=$(((i * 397) % 10800 + 5))
	long=""
	[ $((i % 4)) -eq 0 ] && long=" and a long tail that goes on and on to see where the line is cut: $(printf 'word%.0s ' {1..40})"
	items=()
	for ((k = 0; k < i % 6; k++)); do items+=("item $((k + 1)) of session $i with some text 日本語"); done
	session "$id" "$dir" "$st" "$age" "Objective number $i: do the thing$long" "step $i of many" "Now working on part $i$long" "${items[@]+"${items[@]}"}"
done

# --hostile: text that must never reach the screen as it is
if [ -n "$hostile" ]; then
	esc=$(printf '\033')
	session hostile1 evil working 30 \
		"$(printf 'red \033[31mRED\033[0m clear \033[2J home \033[H')" \
		"$(printf 'title \033]0;PWNED\007 clipboard \033]52;c;ZWNobyBoYWNrZWQ=\007 end')" \
		"$(printf 'bidi \342\200\256gnp.exe\342\200\254 zero\342\200\213width C1 \302\233 31m')" \
		"$(printf 'queued \033[31mred\033[0m')" "$(printf 'x%.0s' {1..5000})"
	session hostile2 "$(printf 'dir\033[31m')" waiting 120 "$(printf 'y%.0s' {1..20000})" "p" "n" "a" "b"
	d="$root/state/sessions/hostile2"
	: >"$d/queue"
	for ((k = 1; k <= 3000; k++)); do echo "item $k" >>"$d/queue"; done
	printf 'Evil Name\033[31m\n' >"$d/alias"
	session hostile3 links waiting 200 "linked files" "p" "n"
	d="$root/state/sessions/hostile3"
	rm -- "$d/queue" "$d/cwd"
	ln -s /etc/hosts "$d/queue"
	ln -s /etc/hosts "$d/cwd"
	session hostile4 junk waiting 300 "binary state" "p" "n"
	printf '\000\001\002 not a state \033[31m\n' >"$root/state/sessions/hostile4/state"
	mkdir -p "$root/state/sessions/bad name" "$root/state/sessions/..evil"
	printf 'Objective: not a session id\n' >"$root/state/sessions/bad name/notes"
	printf '%s\n' "$(printf 'inbox \033[31mred\033[0m')" "$(printf 'z%.0s' {1..3000})" >>"$root/state/inbox.queue"
fi

echo "state:  $root/state"
echo "claude: $root/claude"
