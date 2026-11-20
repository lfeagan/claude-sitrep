#!/bin/bash
# Test suite. Needs only bash and jq. Everything runs against a throwaway $HOME and a stub `claude`,
# so it never touches real settings, real notes or the API.
R=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
export HOME="$T/home"; mkdir -p "$HOME/.claude" "$T/bin"
cat > "$T/bin/claude" <<'STUB'
#!/bin/bash
cat >/dev/null
printf 'Progress: stub progress\nNow: stub now\n'
STUB
chmod +x "$T/bin/claude"
export PATH="$T/bin:$R/bin:$PATH"
NOTES="$HOME/.claude/session-notes"
pass=0; fail=0

check() { # description, then a command whose success means pass
  local d=$1; shift
  if "$@" >/dev/null 2>&1; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL: $d"; fi
}
eq() { # description, expected, actual
  if [ "$2" = "$3" ]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL: $1"; echo "   expected: $2"; echo "   actual:   $3"; fi
}
has() { grep -qF -- "$2" <<<"$1"; }
hasnt() { ! grep -qF -- "$2" <<<"$1"; }

hook() { bash "$R/hooks/notes-hook.sh"; }                                  # reads event JSON on stdin
ev() { jq -nc --arg s "$1" --arg e "$2" --arg p "${3:-}" --arg t "${4:-}" \
         '{session_id:$s,hook_event_name:$e,cwd:"/tmp/proj",prompt:$p,transcript_path:$t}'; }
field() { sed -n "s/^$2: *//p" "$NOTES/$1.txt" | head -1; }
status() { jq -nc --arg s "$1" '{session_id:$s}' | bash "$R/statusline/statusline.sh"; }
waitfor() { for _ in $(seq 1 60); do [ "$(field "$1" "$2")" = "$3" ] && return 0; sleep 0.2; done; return 1; }
mode() { stat -f %Lp "$1" 2>/dev/null || stat -c %a "$1"; }
reset() { rm -rf "$NOTES" "$HOME/.claude/sitrep.conf"; }

# ---- hook: Objective, Now
reset
ev sess0001 UserPromptSubmit "first prompt" | hook
ev sess0001 UserPromptSubmit "second prompt" | hook
eq "objective set from the first prompt and kept" "first prompt" "$(field sess0001 Objective)"
eq "now follows the latest prompt" "second prompt" "$(field sess0001 Now)"

# ---- hook: untrusted text
reset
ev sess0002 UserPromptSubmit $'hi \033[31mRED\033]0;title\a there' | hook
check "escape characters never reach the notes file" bash -c "! LC_ALL=C grep -q \$'\\033' '$NOTES/sess0002.txt'"
eq "text survives with escapes removed" "hi [31mRED]0;title there" "$(field sess0002 Objective)"
ev "../evil" UserPromptSubmit "x" | hook
check "path-like session ids are ignored" test ! -e "$HOME/.claude/evil.txt" -a ! -e "$HOME/.claude/evil.cwd"

# ---- hook: permissions
check "notes dir is owner-only" test "$(mode "$NOTES")" = 700
check "notes file is owner-only" test "$(mode "$NOTES/sess0002.txt")" = 600

# ---- hook: +goal and +
reset
ev sess0003 UserPromptSubmit "long first message" | hook
out=$(ev sess0003 UserPromptSubmit "+goal Ship it" | hook)
eq "+goal blocks the prompt" block "$(jq -r .decision <<<"$out")"
eq "+goal pins the objective" "Ship it" "$(field sess0003 Objective)"
ev sess0003 UserPromptSubmit "another prompt" | hook
eq "pinned objective survives later prompts" "Ship it" "$(field sess0003 Objective)"
out=$(ev sess0003 UserPromptSubmit "+ fix the thing" | hook)
eq "+ blocks the prompt" block "$(jq -r .decision <<<"$out")"
eq "+ queues the item" "fix the thing" "$(head -1 "$NOTES/sess0003.queue")"
eq "+ does not change Now" "another prompt" "$(field sess0003 Now)"

# ---- hook: recursion guard
reset
CLAUDE_NOTES_RUNNING=1 bash -c "$(declare -f ev); ev sess0004 UserPromptSubmit x | bash '$R/hooks/notes-hook.sh'"
check "summarizer recursion guard writes nothing" test ! -e "$NOTES/sess0004.txt"

# ---- hook: SessionStart seeds from the transcript, skipping wrapper messages
reset
TP="$T/transcript.jsonl"
jq -nc '{type:"user",message:{content:"<local-command-caveat>x</local-command-caveat>\n   <command-name>/clear</command-name>\n   <command-message>clear</command-message>"}}' > "$TP"
jq -nc '{type:"user",message:{content:"Debug the flaky test"}}' >> "$TP"
jq -nc '{type:"assistant",message:{content:[{type:"text",text:"on it"}]}}' >> "$TP"
ev sess0005 SessionStart "" "$TP" | hook
eq "resume seeds the objective, skipping the /clear wrapper" "Debug the flaky test" "$(field sess0005 Objective)"
check "summarizer fills Progress" waitfor sess0005 Progress "stub progress"
eq "summarizer fills Now" "stub now" "$(field sess0005 Now)"

# ---- hook: summarizer never overwrites a pinned goal
reset
ev sess0006 UserPromptSubmit "first" | hook
ev sess0006 UserPromptSubmit "+goal Pinned" | hook >/dev/null
ev sess0006 Stop "" "$TP" | hook
check "stop summary arrives" waitfor sess0006 Progress "stub progress"
eq "pinned objective survives the summarizer" "Pinned" "$(field sess0006 Objective)"

# ---- status line
reset
out=$(status sess0007)
check "placeholder objective before the first prompt" has "$out" "Objective: (set on your first prompt)"
check "placeholder queue line" has "$out" "Queue (0) [sess]"
ev sess0007 UserPromptSubmit "hello" | hook
ev sess0007 UserPromptSubmit "+ first item" | hook >/dev/null
out=$(status sess0007)
check "shows objective" has "$out" "Objective: hello"
check "shows queue count and next item" has "$out" "Queue (1) [sess]  next: first item"
printf 'SITREP_HUD_CMD=%q\n' "echo HUDLINE" > "$HOME/.claude/sitrep.conf"
eq "upstream status line runs first" HUDLINE "$(status sess0007 | head -1)"
printf 'Objective: a\033[31mb\033]0;t\a\nProgress: -\nNow: -\n' > "$NOTES/sess0008.txt"
out=$(status sess0008)
check "escapes are stripped at print time" hasnt "$out" $'\033'

# ---- q
reset
ev abcd0001 UserPromptSubmit "obj one" | hook
ev abef0002 UserPromptSubmit "obj two" | hook
check "q add" bash -c "q add abc 'task A' && q add abc 'task B'"
eq "q ls <session> lists the queue" " 1. task A" "$(q ls abc | sed -n 3p)"
eq "q move reports the move" "moved to abef: task A" "$(q move abc abe)"
eq "q done returns and removes the item" "task B" "$(q "done" abc)"
eq "q done on an empty queue fails" 1 "$(q "done" abc >/dev/null 2>&1; echo $?)"
eq "ambiguous prefix fails" 1 "$(q ls ab >/dev/null 2>&1; echo $?)"
eq "non-numeric item is rejected" 1 "$(q "done" abe '1d' >/dev/null 2>&1; echo $?)"
eq "path-like session is rejected" 1 "$(q ls '../x' >/dev/null 2>&1; echo $?)"
q goal abe "New goal" >/dev/null
eq "q goal pins the objective" "New goal" "$(field abef0002 Objective)"
q add abe $'tab\there \033[31mred' >/dev/null
check "q add stores a flat clean line" bash -c "! LC_ALL=C grep -q \$'\\033' '$NOTES/abef0002.queue'"
check "q ls lists sessions" has "$(q ls)" "queue:"

# ---- install / uninstall round trip, in a separate HOME
H2="$T/home2"; mkdir -p "$H2/.claude"
cat > "$H2/.claude/settings.json" <<'JSON'
{"model":"sonnet","statusLine":{"type":"command","command":"echo old-hud"},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo mine"}]}]}}
JSON
cp "$H2/.claude/settings.json" "$T/original.json"
inst() { HOME="$H2" BIN_DIR="$H2/bin" bash "$R/$1"; }
inst install.sh >/dev/null
S2="$H2/.claude/settings.json"
for e in SessionStart UserPromptSubmit Stop; do
  eq "install adds one notes hook for $e" 1 "$(jq "[.hooks.${e}[]|select(.hooks[0].command|contains(\"notes-hook\"))]|length" "$S2")"
done
eq "install keeps the user's own Stop hook" 1 "$(jq '[.hooks.Stop[]|select(.hooks[0].command=="echo mine")]|length' "$S2")"
check "status line points at the repo" jq -e '.statusLine.command|contains("statusline.sh")' "$S2"
# shellcheck source=/dev/null
eq "old status line saved as the upstream" "echo old-hud" "$(. "$H2/.claude/sitrep.conf"; echo "$SITREP_HUD_CMD")"
check "q is linked" test -L "$H2/bin/q"
conf1=$(cat "$H2/.claude/sitrep.conf")
inst install.sh >/dev/null
eq "second install does not duplicate hooks" 1 "$(jq '[.hooks.Stop[]|select(.hooks[0].command|contains("notes-hook"))]|length' "$S2")"
eq "second install keeps the saved upstream" "$conf1" "$(cat "$H2/.claude/sitrep.conf")"
inst uninstall.sh >/dev/null
eq "uninstall restores settings exactly" "$(jq -S . "$T/original.json")" "$(jq -S . "$S2")"
check "uninstall unlinks q" test ! -e "$H2/bin/q"

echo "passed: $pass, failed: $fail"
[ "$fail" -eq 0 ]
