#!/bin/bash
# Test suite. Needs bash, jq, perl and tar. Everything runs against a throwaway $HOME and a stub `claude`,
# so it never touches real settings, real state or the API.
R=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
export HOME="$T/home"; mkdir -p "$HOME/.claude" "$T/bin"
cat > "$T/bin/claude" <<'STUB'
#!/bin/bash
cat >/dev/null
echo x >> "$STUB_CALLS"
[ -e "$STUB_SLOW" ] && sleep 2
printf 'Objective: stub objective\nProgress: stub progress\nNow: stub now\n'
STUB
chmod +x "$T/bin/claude"
export PATH="$T/bin:$R/bin:$PATH" STUB_CALLS="$T/calls" STUB_SLOW="$T/slow"
STATE="$HOME/.claude/sitrep"
pass=0; fail=0

check() { # description, then a command whose success means pass
  local d=$1; shift
  if "$@" >/dev/null 2>&1; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL: $d"; fi
}
eq() { # description, expected, actual
  if [ "$2" = "$3" ]; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL: $1"; echo "   expected: $2"; echo "   actual:   $3"; fi
}
has() { LC_ALL=C grep -qF -- "$2" <<<"$1"; }
hasnt() { ! LC_ALL=C grep -qF -- "$2" <<<"$1"; }
nobytes() { ! LC_ALL=C grep -qF -- "$2" "$1"; }   # file, bytes
calls() { [ -f "$STUB_CALLS" ] && wc -l < "$STUB_CALLS" | tr -d ' ' || echo 0; }

hook() { bash "$R/hooks/sitrep-hook.sh"; }                                  # reads event JSON on stdin
ev() { jq -nc --arg s "$1" --arg e "$2" --arg p "${3:-}" --arg t "${4:-}" \
         '{session_id:$s,hook_event_name:$e,cwd:"/tmp/proj",prompt:$p,transcript_path:$t}'; }
field() { sed -n "s/^$2: *//p" "$STATE/$1.txt" | head -1; }
status_in() { jq -nc --arg s "${2:-sx}" '{session_id:$s}' | HOME="$1" bash "$R/statusline/statusline.sh"; }   # home, session
status() { status_in "$HOME" "$1"; }
rc() { "$@" >/dev/null 2>&1; echo $?; }   # exit status of a command, for use inside eq
waitfor() { for _ in $(seq 1 60); do [ "$(field "$1" "$2")" = "$3" ] && return 0; sleep 0.2; done; return 1; }
mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }
reset() { rm -rf "$STATE" "$HOME/.claude/sitrep/sitrep.conf" "$STUB_CALLS" "$STUB_SLOW"; }
clone() { mkdir -p "$1"; (cd "$R" && tar --exclude=.git -cf - .) | (cd "$1" && tar xf -); }
instc() { HOME="$1" BIN_DIR="$1/bin" bash "$2/$3"; }   # home, repo, script
fresh() { rm -rf "$1"; mkdir -p "$1/.claude"; printf '%s' "$2" > "$1/.claude/settings.json"; }

# ---- hook: Objective, Now
reset
ev sess0001 UserPromptSubmit "first prompt" | hook
ev sess0001 UserPromptSubmit "second prompt" | hook
eq "objective set from the first prompt and kept" "first prompt" "$(field sess0001 Objective)"
eq "now follows the latest prompt" "second prompt" "$(field sess0001 Now)"

# ---- hook: untrusted text
reset
ev sess0002 UserPromptSubmit $'hi \033[31mRED\033]0;title\a there' | hook
check "escape characters never reach the state file" nobytes "$STATE/sess0002.txt" $'\033'
eq "text survives with escapes removed" "hi [31mRED]0;title there" "$(field sess0002 Objective)"
ev "../evil" UserPromptSubmit "x" | hook
check "path-like session ids are ignored" test ! -e "$HOME/.claude/evil.txt" -a ! -e "$HOME/.claude/evil.cwd"

# ---- hook: permissions
check "state dir is owner-only" test "$(mode "$STATE")" = 700
check "state file is owner-only" test "$(mode "$STATE/sess0002.txt")" = 600

# ---- hook: +pin and +
reset
ev sess0003 UserPromptSubmit "long first message" | hook
out=$(ev sess0003 UserPromptSubmit "+pin Ship it" | hook)
eq "+pin blocks the prompt" block "$(jq -r .decision <<<"$out")"
eq "+pin pins the objective" "Ship it" "$(field sess0003 Objective)"
ev sess0003 UserPromptSubmit "another prompt" | hook
eq "pinned objective survives later prompts" "Ship it" "$(field sess0003 Objective)"
out=$(ev sess0003 UserPromptSubmit "+ fix the thing" | hook)
eq "+ blocks the prompt" block "$(jq -r .decision <<<"$out")"
eq "+ queues the item" "fix the thing" "$(head -1 "$STATE/sess0003.queue")"
eq "+ does not change Now" "another prompt" "$(field sess0003 Now)"

# ---- hook: recursion guard
reset
SITREP_SUMMARIZING=1 bash -c "$(declare -f ev); ev sess0004 UserPromptSubmit x | bash '$R/hooks/sitrep-hook.sh'"
check "summarizer recursion guard writes nothing" test ! -e "$STATE/sess0004.txt"

# ---- hook: SessionStart seeds from the transcript, skipping wrapper messages
reset
mkdir -p "$HOME/.claude/projects/p"; TP="$HOME/.claude/projects/p/transcript.jsonl"
jq -nc '{type:"user",message:{content:"<local-command-caveat>x</local-command-caveat>\n   <command-name>/clear</command-name>\n   <command-message>clear</command-message>"}}' > "$TP"
jq -nc '{type:"user",message:{content:"Debug the flaky test"}}' >> "$TP"
jq -nc '{type:"assistant",message:{content:[{type:"text",text:"on it"}]}}' >> "$TP"
touch "$STUB_SLOW"; ev sess0005 SessionStart "" "$TP" | hook
eq "resume seeds the objective, skipping the /clear wrapper" "Debug the flaky test" "$(field sess0005 Objective)"
check "summarizer fills Progress" waitfor sess0005 Progress "stub progress"
eq "summarizer fills Now" "stub now" "$(field sess0005 Now)"
eq "summarizer replaces an unpinned objective" "stub objective" "$(field sess0005 Objective)"
rm -f "$STUB_SLOW"

# ---- hook: summarizer never overwrites a pinned goal
reset
ev sess0006 UserPromptSubmit "first" | hook
ev sess0006 UserPromptSubmit "+pin Pinned" | hook >/dev/null
ev sess0006 Stop "" "$TP" | hook
check "stop summary arrives" waitfor sess0006 Progress "stub progress"
eq "pinned objective survives the summarizer" "Pinned" "$(field sess0006 Objective)"
reset
ev sess0007 UserPromptSubmit "first" | hook
ev sess0007 Stop "" "$TP" | hook
check "stop summary arrives for an unpinned session" waitfor sess0007 Progress "stub progress"
eq "an unpinned objective is summarized" "stub objective" "$(field sess0007 Objective)"
reset
ev sess0009 UserPromptSubmit "first" | hook
sitrep pin sess "Pinned by cli" >/dev/null
ev sess0009 Stop "" "$TP" | hook
check "stop summary arrives after sitrep pin" waitfor sess0009 Progress "stub progress"
eq "sitrep pin pins against the summarizer" "Pinned by cli" "$(field sess0009 Objective)"
sitrep unpin sess >/dev/null; rm -f "$STATE/.sess0009.stamp"
ev sess0009 Stop "" "$TP" | hook
check "sitrep unpin lets the summarizer rewrite the objective" waitfor sess0009 Objective "stub objective"

# ---- hook: C1 controls and bidi overrides are untrusted too
reset
ev sess0011 UserPromptSubmit "$(printf 'hi \xc2\x9b31mRED\xc2\x9d0;t\xc2\x9c \xe2\x80\xaeX')" | hook
check "C1 CSI removed" nobytes "$STATE/sess0011.txt" $'\xc2\x9b'
check "C1 OSC removed" nobytes "$STATE/sess0011.txt" $'\xc2\x9d'
check "C1 ST removed" nobytes "$STATE/sess0011.txt" $'\xc2\x9c'
check "bidi override removed" nobytes "$STATE/sess0011.txt" $'\xe2\x80\xae'
eq "ordinary text and UTF-8 survive" "héllo 世界" "$(ev sess0012 UserPromptSubmit 'héllo 世界' | hook; field sess0012 Objective)"
printf 'Objective: a\xc2\x9b31m\xe2\x80\xaeb\nProgress: -\nNow: -\n' > "$STATE/sess0013.txt"
out=$(status sess0013)
check "status line drops C1 and bidi at print time" hasnt "$out" $'\xc2\x9b'
check "status line drops bidi at print time" hasnt "$out" $'\xe2\x80\xae'
check "sitrep ls drops C1 at print time" hasnt "$(sitrep ls)" $'\xc2\x9b'

# ---- hook: + and +pin never lose a prompt
reset
ev sess0020 UserPromptSubmit "start" | hook
out=$(ev sess0020 UserPromptSubmit "+ $(printf 'x%.0s' $(seq 1 300))" | hook)
eq "long + prompt passes through" "" "$out"
out=$(ev sess0020 UserPromptSubmit $'+ line one\nline two' | hook)
eq "multi-line + prompt passes through" "" "$out"
out=$(ev sess0020 UserPromptSubmit "+pin $(printf 'y%.0s' $(seq 1 300))" | hook)
eq "long +pin prompt passes through" "" "$out"
check "nothing was queued" test ! -s "$STATE/sess0020.queue"
mkdir "$STATE/sess0020.queue.d"; rm -f "$STATE/sess0020.queue"; mkdir "$STATE/sess0020.queue"
out=$(ev sess0020 UserPromptSubmit "+ important" | hook 2>/dev/null)
eq "unwritable queue passes the prompt through" "" "$out"
rmdir "$STATE/sess0020.queue" "$STATE/sess0020.queue.d"

# ---- hook: label padding, blank items, deferred debounce
reset
for i in 1 2 3; do ev sess0040 UserPromptSubmit "p$i" | hook; done
eq "label padding does not grow across prompts" "Progress:  -" "$(grep '^Progress:' "$STATE/sess0040.txt")"
out=$(ev sess0040 UserPromptSubmit "+ $(printf '\001\002')" | hook)
eq "a + item that cleans to nothing passes through" "" "$out"
check "no blank queue line was written" test ! -s "$STATE/sess0040.queue"

reset; export SITREP_DEBOUNCE=2
ev sess0041 UserPromptSubmit "first" | hook
ev sess0041 Stop "" "$TP" | hook
ev sess0041 Stop "" "$TP" | hook
ev sess0041 Stop "" "$TP" | hook
check "first Stop summarizes" waitfor sess0041 Progress "stub progress"
eq "Stops inside the window are held, not run" 1 "$(calls)"
for _ in $(seq 1 50); do [ "$(calls)" = 2 ] && break; sleep 0.1; done
eq "one deferred summary runs at the end of the window" 2 "$(calls)"
unset SITREP_DEBOUNCE

# ---- hook: debounce, stale summaries, transcript location
reset
ev sess0030 UserPromptSubmit "first" | hook
ev sess0030 Stop "" "$TP" | hook
ev sess0030 Stop "" "$TP" | hook
check "summary arrives" waitfor sess0030 Progress "stub progress"
eq "second Stop inside 45s does not call the model" 1 "$(calls)"
rm -f "$STUB_CALLS"
ev sess0031 Stop "" "$T/outside.jsonl" | hook
sleep 0.3
eq "transcripts outside the projects directory are ignored" 0 "$(calls)"
rm -f "$STUB_CALLS"; touch "$STUB_SLOW"
ev sess0032 UserPromptSubmit "first" | hook
ev sess0032 Stop "" "$TP" | hook
ev sess0032 UserPromptSubmit "newer prompt" | hook
sleep 3
eq "a stale summary does not overwrite a newer Now" "newer prompt" "$(field sess0032 Now)"
rm -f "$STUB_SLOW"

# ---- status line
reset
out=$(status sess0007)
check "placeholder objective before the first prompt" has "$out" "Objective: (set on your first prompt)"
check "placeholder queue line" has "$out" "Queue: empty  [id sess]"
ev sess0007 UserPromptSubmit "hello" | hook
ev sess0007 UserPromptSubmit "+ first item" | hook >/dev/null
out=$(status sess0007)
check "shows objective" has "$out" "Objective: hello"
check "shows queue count and next item" has "$out" "Queue: 1 item, next: first item  [id sess]"
mkdir -p "$HOME/.claude/sitrep"; printf 'SITREP_HUD_CMD=%q\n' "echo HUDLINE" > "$HOME/.claude/sitrep/sitrep.conf"
eq "upstream status line runs first" HUDLINE "$(status sess0007 | head -1)"
printf 'Objective: a\033[31mb\033]0;t\a\nProgress: -\nNow: -\n' > "$STATE/sess0008.txt"
out=$(status sess0008)
check "escapes are stripped at print time" hasnt "$out" $'\033'

# ---- q
reset
ev abcd0001 UserPromptSubmit "obj one" | hook
ev abef0002 UserPromptSubmit "obj two" | hook
check "sitrep add" bash -c "sitrep add abc 'task A' && sitrep add abc 'task B'"
eq "sitrep ls <session> lists the queue" " 1. task A" "$(sitrep ls abc | sed -n 3p)"
eq "sitrep move reports the move" "moved to abef: task A" "$(sitrep move abc abe)"
eq "sitrep rm returns and removes the item" "task B" "$(sitrep rm abc)"
eq "sitrep rm on an empty queue fails" 1 "$(rc sitrep rm abc)"
eq "ambiguous prefix fails" 1 "$(rc sitrep ls ab)"
eq "non-numeric item is rejected" 1 "$(rc sitrep rm abe '1d')"
eq "path-like session is rejected" 1 "$(rc sitrep ls '../x')"
sitrep pin abe "New goal" >/dev/null
eq "sitrep pin pins the objective" "New goal" "$(field abef0002 Objective)"
sitrep add abe $'tab\there \033[31mred\xc2\x9b31m' >/dev/null
check "sitrep add strips escapes" nobytes "$STATE/abef0002.queue" $'\033'
check "sitrep add strips C1 controls" nobytes "$STATE/abef0002.queue" $'\xc2\x9b'
check "sitrep ls has a header" has "$(sitrep ls)" "ID    DIRECTORY"
check "sitrep ls shows the item count and next item" has "$(sitrep ls)" "2 items, next: task A"
mkdir -p "$STATE"; printf 'Objective: wide\n' > "$STATE/wide0001.txt"; printf '/x/a-very-long-directory-name\n' > "$STATE/wide0001.cwd"
cols=$(sitrep ls | grep -E '^(ID|wide|abef)' | awk '{print index($0,"empty") + index($0,"2 items") + index($0,"QUEUE")}' | sort -u | wc -l | tr -d ' ')
eq "sitrep ls aligns the queue column for long directory names" 1 "$cols"
rm -f "$STATE/wide0001".*
printf 'Objective: cut\n' > "$STATE/cutq0001.txt"; sitrep add cutq "$(printf 'z%.0s' $(seq 1 150))" >/dev/null
row=$(COLUMNS=60 sitrep ls | grep -E '^cutq')
rm -f "$STATE/cutq0001".*
check "sitrep ls cuts a long next item to COLUMNS" test "$(printf '%s' "$row" | perl -CSD -ne 'print length')" -le 60
check "sitrep ls marks the cut with an ellipsis" has "$row" "…"
check "sitrep ls says empty for an empty queue" has "$(sitrep ls)" "empty"
printf 'Objective: only\n' > "$STATE/abcd0001.txt"
sitrep pin abcd "replaced" >/dev/null
eq "sitrep pin works on an objective-only file" "replaced" "$(field abcd0001 Objective)"
sitrep add abcd one >/dev/null; sitrep rm abcd >/dev/null
check "sitrep ls prints no stray count for an emptied queue" bash -c '! sitrep ls | grep -qx 0'
eq "sitrep add reports a zero count once" "queued on abcd (now 1)" "$(sitrep add abcd two)"
eq "sitrep rm past the end of the queue fails" 1 "$(rc sitrep rm abcd 9)"
eq "sitrep add rejects text that cleans to nothing" 1 "$(rc sitrep add abcd "$(printf '\001')")"

# ---- hook: +inbox and +take
rm -f "$STATE/inbox.queue" "$STATE/tk000001.queue"
out=$(ev tk000001 UserPromptSubmit "+inbox look at the CI flake" | hook)
eq "+inbox blocks the prompt" block "$(jq -r .decision <<<"$out")"
eq "+inbox reports the inbox count" "Inbox (1): look at the CI flake" "$(jq -r .reason <<<"$out")"
eq "+inbox writes to the inbox" "look at the CI flake" "$(head -1 "$STATE/inbox.queue")"
check "+inbox leaves the session queue alone" test ! -s "$STATE/tk000001.queue"
eq "+inbox passes a long prompt through" "" "$(ev tk000001 UserPromptSubmit "+inbox $(printf 'y%.0s' $(seq 1 300))" | hook)"
eq "+inbox passes a multi-line prompt through" "" "$(ev tk000001 UserPromptSubmit $'+inbox one\ntwo' | hook)"
sitrep add inbox 'second item' >/dev/null; sitrep add inbox 'third item' >/dev/null
out=$(ev tk000001 UserPromptSubmit "+take" | hook)
eq "+take blocks the prompt" block "$(jq -r .decision <<<"$out")"
eq "+take reports the item" "Took from the inbox: look at the CI flake" "$(jq -r .reason <<<"$out")"
eq "+take puts the item on this session" "look at the CI flake" "$(head -1 "$STATE/tk000001.queue")"
eq "+take removes it from the inbox" 2 "$(grep -c . "$STATE/inbox.queue")"
ev tk000001 UserPromptSubmit "+take 2" | hook >/dev/null
eq "+take N takes item N" "third item" "$(tail -1 "$STATE/tk000001.queue")"
eq "+take leaves the rest" "second item" "$(cat "$STATE/inbox.queue")"
eq "+take with text after it goes to Claude" "" "$(ev tk000001 UserPromptSubmit "+take care of the rest" | hook)"
eq "+take past the end goes to Claude" "" "$(ev tk000001 UserPromptSubmit "+take 9" | hook)"
eq "a failed +take changed nothing" "second item" "$(cat "$STATE/inbox.queue")"
: > "$STATE/inbox.queue"
eq "+take on an empty inbox goes to Claude" "" "$(ev tk000001 UserPromptSubmit "+take" | hook)"
ev tk000001 UserPromptSubmit "+inbox    padded" | hook >/dev/null
eq "+inbox drops leading spaces" "padded" "$(tail -1 "$STATE/inbox.queue")"
: > "$STATE/inbox.queue"; printf 'a\tb\xe2\x80\xa8c\n' > "$STATE/inbox.queue"
out=$(ev tk000001 UserPromptSubmit "+take" | hook)
eq "+take prints the item without tabs or line separators" "Took from the inbox: a b c" "$(jq -r .reason <<<"$out")"
printf 'keep\n' > "$STATE/inbox.queue"; : > "$STATE/tk000001.queue"; chmod 400 "$STATE/tk000001.queue"
if [ ! -w "$STATE/tk000001.queue" ]; then   # root ignores the mode
  eq "+take that cannot write the session queue goes to Claude" "" "$(ev tk000001 UserPromptSubmit "+take" | hook)"
  eq "a failed +take does not lose the inbox item" keep "$(cat "$STATE/inbox.queue")"
fi
chmod 600 "$STATE/tk000001.queue"
rm -f "$STATE/inbox.queue" "$STATE"/tk000001.* "$STATE"/.tk000001.*

# ---- reorder
printf 'Objective: r\n' > "$STATE/rord0001.txt"; rm -f "$STATE/rord0001.queue"
for w in one two three four; do sitrep add rord "$w" >/dev/null; done
eq "top moves an item to the front" "item 1 on rord: three" "$(sitrep top rord 3)"
eq "top gives the new order" "three one two four" "$(tr '\n' ' ' < "$STATE/rord0001.queue" | sed 's/ $//')"
sitrep reorder rord 1 4 >/dev/null
eq "reorder moves an item to a later position" "one two four three" "$(tr '\n' ' ' < "$STATE/rord0001.queue" | sed 's/ $//')"
sitrep reorder rord 4 2 >/dev/null
eq "reorder moves an item to an earlier position" "one three two four" "$(tr '\n' ' ' < "$STATE/rord0001.queue" | sed 's/ $//')"
sitrep reorder rord 2 2 >/dev/null
eq "reorder to the same position changes nothing" "one three two four" "$(tr '\n' ' ' < "$STATE/rord0001.queue" | sed 's/ $//')"
eq "reorder past the end fails" 1 "$(rc sitrep reorder rord 1 9)"
eq "top of a missing item fails" 1 "$(rc sitrep top rord 9)"
eq "reorder rejects a non-number" 1 "$(rc sitrep reorder rord x 1)"
eq "a failed reorder changed nothing" "one three two four" "$(tr '\n' ' ' < "$STATE/rord0001.queue" | sed 's/ $//')"
sitrep add inbox alpha >/dev/null; sitrep add inbox beta >/dev/null; sitrep top inbox 2 >/dev/null
eq "top works on the inbox" beta "$(head -1 "$STATE/inbox.queue")"
rm -f "$STATE/inbox.queue" "$STATE"/rord000*

# ---- inbox: the queue of no session
rm -f "$STATE/inbox.queue"
printf 'Objective: a\n' > "$STATE/inba0001.txt"; printf 'Objective: b\n' > "$STATE/inbb0002.txt"
eq "add to the inbox" "queued on inbox (now 1)" "$(sitrep add inbox 'first thing')"
sitrep add inbox 'second thing' >/dev/null
check "ls inbox lists numbered items" has "$(sitrep ls inbox)" " 2. second thing"
check "ls shows an Inbox line" has "$(sitrep ls)" "Inbox: 2 items, next: first thing"
eq "move inbox to a session" "moved to inba: first thing" "$(sitrep move inbox inba 1)"
eq "the item left the inbox" 1 "$(grep -c . "$STATE/inbox.queue")"
eq "the item reached the session" "first thing" "$(head -1 "$STATE/inba0001.queue")"
eq "move a session item into the inbox" "moved to inbox: first thing" "$(sitrep move inba inbox)"
eq "move to the same queue fails" 1 "$(rc sitrep move inbox inbox)"
eq "the same-queue move changed nothing" "second thing" "$(head -1 "$STATE/inbox.queue")"
eq "move from an empty queue fails" 1 "$(rc sitrep move inba inbox)"
eq "pin on the inbox fails" 1 "$(rc sitrep pin inbox x)"
eq "unpin on the inbox fails" 1 "$(rc sitrep unpin inbox)"
check "no ghost session files for the inbox" test ! -e "$STATE/inbox.txt" -a ! -e "$STATE/inbox.cwd" -a ! -e "$STATE/.inbox.pinned"
eq "a prefix of inbox is not the inbox" 1 "$(rc sitrep add inb x)"
check "status line shows the inbox count" has "$(status inba0001)" "Inbox: 2"
printf 'a\nb\n' > "$STATE/inbox.queue"; sitrep rm inbox >/dev/null; sitrep rm inbox >/dev/null
check "status line hides an empty inbox" hasnt "$(status inba0001)" "Inbox"
check "ls prints no Inbox line when empty" hasnt "$(sitrep ls)" "Inbox:"
printf 'x\n' > "$STATE/inbox.queue"
check "status line ignores inbox as a session id" test -z "$(status inbox)"
rm -f "$STATE/inbox.queue"
printf 'one\n' > "$STATE/inbox.queue"
fn=$(sed -n '/^without()/,/^}/p' "$R/bin/sitrep")
eq "a stale item number is refused, not mis-deleted" 1 "$(rc bash -c "tmpfile() { mktemp '$STATE/.sitrep.XXXXXX'; }; $fn; without '$STATE/inbox.queue' 1 'other'")"
eq "the refused delete left the queue alone" one "$(cat "$STATE/inbox.queue")"
printf 'esc\033[31m\n' > "$STATE/inbox.queue"
check "ls inbox strips escapes" hasnt "$(sitrep ls inbox)" $'\033'
ev inbox UserPromptSubmit hi | hook
check "a session id of inbox is ignored by the hook" test ! -e "$STATE/inbox.txt" -a ! -e "$STATE/inbox.cwd"
rm -f "$STATE/inbox.queue" "$STATE"/inb[ab]000*

# ---- install / uninstall round trip, in a separate HOME
H2="$T/home2"
fresh "$H2" '{"model":"sonnet","statusLine":{"type":"command","command":"echo old-hud"},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo mine"}]}]}}'
cp "$H2/.claude/settings.json" "$T/original.json"
instc "$H2" "$R" install.sh >/dev/null
S2="$H2/.claude/settings.json"
for e in SessionStart UserPromptSubmit Stop; do
  eq "install adds one hook for $e" 1 "$(jq "[.hooks.${e}[]|select(.hooks[0].command|contains(\"sitrep-hook\"))]|length" "$S2")"
done
eq "install keeps the user's own Stop hook" 1 "$(jq '[.hooks.Stop[]|select(.hooks[0].command=="echo mine")]|length' "$S2")"
check "status line points at the repo" jq -e '.statusLine.command|contains("statusline.sh")' "$S2"
# shellcheck source=/dev/null
eq "old status line saved as the upstream" "echo old-hud" "$(. "$H2/.claude/sitrep/sitrep.conf"; echo "$SITREP_HUD_CMD")"
check "sitrep is linked" test -L "$H2/bin/sitrep"
check "skill is linked" test -L "$H2/.claude/skills/sitrep-queue" -a -f "$H2/.claude/skills/sitrep-queue/SKILL.md"
conf1=$(cat "$H2/.claude/sitrep/sitrep.conf")
instc "$H2" "$R" install.sh >/dev/null
eq "second install does not duplicate hooks" 1 "$(jq '[.hooks.Stop[]|select(.hooks[0].command|contains("sitrep-hook"))]|length' "$S2")"
eq "second install keeps the saved upstream" "$conf1" "$(cat "$H2/.claude/sitrep/sitrep.conf")"
instc "$H2" "$R" uninstall.sh >/dev/null
eq "uninstall restores settings exactly" "$(jq -S . "$T/original.json")" "$(jq -S . "$S2")"
check "uninstall unlinks sitrep" test ! -e "$H2/bin/sitrep"
check "uninstall unlinks the skill" test ! -e "$H2/.claude/skills/sitrep-queue" -a ! -L "$H2/.claude/skills/sitrep-queue"

# ---- install: clones, output and recovery

# an install from a second clone must not save our own status line as the upstream
H3="$T/home3"; fresh "$H3" '{"statusLine":{"type":"command","command":"echo old-hud"}}'
clone "$T/clone-a"; clone "$T/clone-b"
instc "$H3" "$T/clone-a" install.sh >/dev/null; instc "$H3" "$T/clone-b" install.sh >/dev/null 2>&1
check "upstream stays the original after installs from two clones" bash -c "grep -q 'old-hud' '$H3/.claude/sitrep/sitrep.conf' && ! grep -q 'statusline.sh' '$H3/.claude/sitrep/sitrep.conf'"
eq "status line refuses to render itself" "" "$(jq -nc '{session_id:"x"}' | SITREP_RENDERING=1 bash "$R/statusline/statusline.sh")"

# install output says what it found
H9="$T/home9"; fresh "$H9" '{}'
check "install reports no existing status line" has "$(instc "$H9" "$R" install.sh 2>&1)" "No existing status line"
check "no sitrep.conf is written without an upstream" test ! -e "$H9/.claude/sitrep/sitrep.conf"
check "block renders without an upstream" has "$(status_in "$H9")" "Queue: empty"
H10="$T/home10"; fresh "$H10" '{"statusLine":{"type":"command","command":"echo hud"}}'
check "install reports the status line it wraps" has "$(instc "$H10" "$R" install.sh 2>&1)" "Wrapping the existing status line (it renders first): echo hud"
eq "wrapped upstream renders first" hud "$(status_in "$H10" | head -1)"
# a HUD setup that overwrites our status line is repaired by re-running install
jq '.statusLine.command="echo hud-again"' "$H10/.claude/settings.json" > "$T/h10.json" && cp "$T/h10.json" "$H10/.claude/settings.json"
instc "$H10" "$R" install.sh >/dev/null 2>&1
eq "re-install after an overwrite wraps the new command" hud-again "$(status_in "$H10" | head -1)"
check "sitrep block is back after the overwrite" has "$(status_in "$H10")" "Queue: empty"

# ---- install: stale config, look-alike status line, failed merge, upstream without a newline
H11="$T/home11"; fresh "$H11" '{}'; mkdir -p "$H11/.claude/sitrep"; printf 'SITREP_HUD_CMD=%q\n' "echo stale" > "$H11/.claude/sitrep/sitrep.conf"
instc "$H11" "$R" install.sh >/dev/null
out=$(status_in "$H11")
check "a stale upstream is cleared when there is no status line" hasnt "$out" "stale"
H12="$T/home12"; fresh "$H12" '{"statusLine":{"type":"command","command":"/opt/other/statusline/statusline.sh"}}'
instc "$H12" "$R" install.sh >/dev/null 2>&1
check "a foreign statusline.sh is wrapped, not mistaken for ours" grep -q "other/statusline" "$H12/.claude/sitrep/sitrep.conf"
instc "$H12" "$R" uninstall.sh >/dev/null
eq "uninstall restores the foreign status line" "/opt/other/statusline/statusline.sh" "$(jq -r .statusLine.command "$H12/.claude/settings.json")"
check "uninstall removes the consumed upstream config" test ! -e "$H12/.claude/sitrep/sitrep.conf"
H13="$T/home13"; fresh "$H13" '{"hooks":5,"statusLine":{"type":"command","command":"echo hud"}}'
out=$(instc "$H13" "$R" install.sh 2>&1)
check "a failed merge writes no upstream config" test ! -e "$H13/.claude/sitrep/sitrep.conf"
check "a failed merge does not claim to wrap anything" hasnt "$out" "Wrapping"
mkdir -p "$HOME/.claude/sitrep"; printf 'SITREP_HUD_CMD=%q\n' "printf hud" > "$HOME/.claude/sitrep/sitrep.conf"
eq "upstream without a trailing newline does not glue onto our block" "hud" "$(status sess0007 | head -1)"
rm -f "$HOME/.claude/sitrep/sitrep.conf"

# paths with spaces
H4="$T/home4"; fresh "$H4" '{}'
clone "$T/my dir/sitrep"
instc "$H4" "$T/my dir/sitrep" install.sh >/dev/null
cmd=$(jq -r '.hooks.Stop[-1].hooks[0].command' "$H4/.claude/settings.json")
eq "stored hook command runs from a path with spaces" 0 "$(echo '{}' | HOME="$H4" bash -c "$cmd" >/dev/null 2>&1; echo $?)"
slc=$(jq -r '.statusLine.command' "$H4/.claude/settings.json")
check "stored status line command runs from a path with spaces" has "$(jq -nc '{session_id:"sx"}' | HOME="$H4" bash -c "$slc")" "Queue: empty"

# hooks we do not own: non-command hooks, empty groups, look-alike names
H5="$T/home5"; fresh "$H5" '{"hooks":{"Stop":[{"hooks":[{"type":"prompt","prompt":"x"}]},{"hooks":[]},{"hooks":[{"type":"command","command":"my-notes-hook-other.sh"}]}]},"statusLine":{"type":"command","command":"x","padding":2}}'
cp "$H5/.claude/settings.json" "$T/h5.json"
check "install tolerates non-command hooks and empty groups" instc "$H5" "$R" install.sh
eq "install keeps the user's look-alike hook" 1 "$(jq '[.hooks.Stop[]|select(.hooks[0].command=="my-notes-hook-other.sh")]|length' "$H5/.claude/settings.json")"
eq "install keeps statusLine fields it does not own" 2 "$(jq .statusLine.padding "$H5/.claude/settings.json")"
check "our hook is installed" jq -e '[.hooks.Stop[]|select((.hooks[0].command // "")|contains("sitrep-hook.sh"))]|length==1' "$H5/.claude/settings.json"
instc "$H5" "$R" uninstall.sh >/dev/null
eq "uninstall restores settings with foreign hooks exactly" "$(jq -S . "$T/h5.json")" "$(jq -S . "$H5/.claude/settings.json")"

# a q we do not own
H6="$T/home6"; fresh "$H6" '{}'; mkdir -p "$H6/bin"; echo "mine" > "$H6/bin/sitrep"
instc "$H6" "$R" install.sh >/dev/null 2>&1
eq "install leaves a foreign sitrep alone" mine "$(cat "$H6/bin/sitrep")"
instc "$H6" "$R" uninstall.sh >/dev/null
eq "uninstall leaves a foreign sitrep alone" mine "$(cat "$H6/bin/sitrep")"

# a symlinked settings.json stays a symlink
H7="$T/home7"; fresh "$H7" ''; mkdir -p "$T/dotfiles"; echo '{}' > "$T/dotfiles/settings.json"
rm "$H7/.claude/settings.json"; ln -s "$T/dotfiles/settings.json" "$H7/.claude/settings.json"
instc "$H7" "$R" install.sh >/dev/null
check "settings.json is still a symlink" test -L "$H7/.claude/settings.json"
check "the symlink target received the hooks" jq -e '.hooks.Stop|length==1' "$T/dotfiles/settings.json"

# a failure must not report success
H8="$T/home8"; fresh "$H8" '{not json'
check "install fails loudly on unparseable settings" bash -c "! HOME='$H8' BIN_DIR='$H8/bin' bash '$R/install.sh' >/dev/null 2>&1"
eq "a failed install leaves settings untouched" "{not json" "$(cat "$H8/.claude/settings.json")"
check "a failed install leaves no temp files" bash -c "! ls '$H8/.claude' | grep -q 'settings.json\\.[A-Za-z0-9]\\{6\\}'"

# alias, migration from the old locations
H14="$T/home14"; fresh "$H14" '{"statusLine":{"type":"command","command":"bash /x/statusline.sh # claude-sitrep"}}'; mkdir -p "$H14/.claude/session-notes"
echo "Objective: kept" > "$H14/.claude/session-notes/abcd0001.txt"; echo 'SITREP_HUD_CMD=old' > "$H14/.claude/sitrep.conf"
HOME="$H14" BIN_DIR="$H14/bin" bash "$R/install.sh" --alias q >/dev/null 2>&1
check "install --alias links the alias" test "$(readlink "$H14/bin/q")" = "$R/bin/sitrep"
check "install moves old state" test -f "$H14/.claude/sitrep/abcd0001.txt" -a ! -e "$H14/.claude/session-notes"
check "install moves the old config into the state folder" test -f "$H14/.claude/sitrep/sitrep.conf" -a ! -e "$H14/.claude/sitrep.conf"
instc "$H14" "$R" uninstall.sh >/dev/null
check "uninstall removes the alias" test ! -e "$H14/bin/q" -a ! -L "$H14/bin/q"
H19="$T/home19"; fresh "$H19" '{}'; mkdir -p "$H19/bin"; ln -s "$R/bin/q" "$H19/bin/q"
instc "$H19" "$R" install.sh >/dev/null 2>&1
check "install removes a dangling old q link from this checkout" test ! -L "$H19/bin/q"
H20="$T/home20"; fresh "$H20" '{}'; mkdir -p "$H20/bin"; ln -s /elsewhere/q "$H20/bin/q"
instc "$H20" "$R" install.sh >/dev/null 2>&1
check "install leaves a dangling q link that is not ours" test -L "$H20/bin/q"
H17="$T/home17"; fresh "$H17" '{}'; mkdir -p "$H17/.claude/session-notes" "$H17/.claude/sitrep"
echo old > "$H17/.claude/session-notes/abcd0001.txt"; echo old > "$H17/.claude/session-notes/ijkl0003.txt"; echo new > "$H17/.claude/sitrep/abcd0001.txt"
instc "$H17" "$R" install.sh >/dev/null 2>&1
check "install merges old state into an existing folder" test -f "$H17/.claude/sitrep/ijkl0003.txt" -a ! -e "$H17/.claude/session-notes/ijkl0003.txt"
eq "merge never overwrites newer state" new "$(cat "$H17/.claude/sitrep/abcd0001.txt")"
H15="$T/home15"; fresh "$H15" '{}'; mkdir -p "$H15/.claude/session-notes"; echo x > "$H15/.claude/session-notes/other.dat"
instc "$H15" "$R" install.sh >/dev/null 2>&1
check "install leaves a state folder with foreign files alone" test -f "$H15/.claude/session-notes/other.dat"
H16="$T/home16"; fresh "$H16" '{}'; mkdir -p "$H16/bin"; echo mine > "$H16/bin/q"
HOME="$H16" BIN_DIR="$H16/bin" bash "$R/install.sh" --alias q >/dev/null 2>&1
eq "install --alias leaves a foreign command alone" mine "$(cat "$H16/bin/q")"
check "install rejects a bad alias" bash -c "! HOME='$H16' bash '$R/install.sh' --alias '../x' >/dev/null 2>&1"

# upgrade from the old hook name
H18="$T/home18"; fresh "$H18" "$(jq -nc --arg c "bash $R/hooks/notes-hook.sh" '{hooks:{Stop:[{hooks:[{type:"command",command:$c}]}]}}')"
instc "$H18" "$R" install.sh >/dev/null 2>&1
eq "install replaces the old hook name" 1 "$(jq '[.hooks.Stop[]]|length' "$H18/.claude/settings.json")"
check "the remaining hook is sitrep-hook.sh" jq -e '.hooks.Stop[0].hooks[0].command|contains("sitrep-hook.sh")' "$H18/.claude/settings.json"
fresh "$H18" "$(jq -nc --arg c "bash $R/hooks/notes-hook.sh" '{hooks:{Stop:[{hooks:[{type:"command",command:$c}]}]}}')"
instc "$H18" "$R" uninstall.sh >/dev/null 2>&1
check "uninstall also removes the old hook name" jq -e 'has("hooks")|not' "$H18/.claude/settings.json"

echo "passed: $pass, failed: $fail"
[ "$fail" -eq 0 ]
