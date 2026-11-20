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

# ---- watch and ls -v
reset; mkdir -p "$STATE"
bounded() { perl -e 'alarm 10; exec @ARGV' -- "$@"; }   # a looping watch fails the test instead of hanging it
mk() { # id dir age: a session with Objective, Progress and Now, last touched age seconds ago
  printf 'Objective: obj %s\nProgress:  prog %s\nNow:       now %s\n' "$1" "$1" "$1" > "$STATE/$1.txt"
  printf '/x/%s\n' "$2" > "$STATE/$1.cwd"; perl -e '$t = time - shift; utime $t, $t, @ARGV' "$3" "$STATE/$1.txt"
}
mk dsh10001 alpha 500; mk dsh10002 beta 400; mk dsh10003 gamma 300
mk dsh10004 delta 200; mk dsh10005 eps 100; mk dsh10006 ancient 200000
eq "ls exits 0" 0 "$(rc sitrep ls)"
check "ls hides Progress" hasnt "$(sitrep ls)" "Progress:"
check "ls -v shows Progress" has "$(sitrep ls -v)" "Progress:  prog dsh1"
check "ls leaves out a session idle for more than a day" hasnt "$(sitrep ls -v)" "ancient"
eq "ls lists the newest session first" dsh1 "$(sitrep ls | sed -n 2p | cut -c1-4)"
eq "newest first by modification time" "obj dsh10005" "$(sitrep ls | sed -n 3p | sed 's/.*Objective: //')"
check "watch prints one frame and exits when stdout is not a terminal" has "$(bounded sitrep watch)" "Progress:  prog dsh1"
eq "watch exits 0 here" 0 "$(rc bounded sitrep watch)"
eq "watch --once exits 0" 0 "$(rc bounded sitrep watch --once)"
out=$(LINES=10 COLUMNS=80 bounded sitrep watch --once)
eq "a tall frame is cut to LINES" 10 "$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
eq "the cut frame ends in a footer" "+3 more" "$(printf '%s\n' "$out" | tail -1)"
check "whole sessions are kept, none half shown" has "$out" "now dsh10004"
eq "a frame that fits has no footer" 0 "$(LINES=40 COLUMNS=80 bounded sitrep watch --once | grep -c 'more$')"
out=$(LINES=40 COLUMNS=30 bounded sitrep watch --once)
eq "watch cuts every line to COLUMNS" 0 "$(printf '%s\n' "$out" | perl -CSD -ne 'chomp; print "x\n" if length > 30' | wc -l | tr -d ' ')"
printf 'Objective: tab\there\033[31mred\nProgress:  p\nNow:       n\n' > "$STATE/dsh10005.txt"
out=$(LINES=40 COLUMNS=80 bounded sitrep watch --once)
check "watch strips escapes" hasnt "$out" $'\033'
check "watch turns tabs into spaces" hasnt "$out" $'\t'
sitrep add inbox 'inbox thing' >/dev/null
eq "watch puts the Inbox first, as a count and a list" "Inbox: 1 item|    1. inbox thing" "$(LINES=40 COLUMNS=80 bounded sitrep watch --once | head -2 | paste -sd'|' -)"
eq "plain ls keeps the one-line Inbox" "Inbox: 1 item, next: inbox thing" "$(sitrep ls | head -1)"
rm -f "$STATE/inbox.queue"
fn=$(sed -n '/^sig()/,/^}/p' "$R/bin/sitrep")
sigof() { bash -c "dir='$STATE'; $fn; sig"; }
before=$(sigof); printf 'x\n' >> "$STATE/dsh10001.queue"
check "the change signature sees an in-place append" test "$before" != "$(sigof)"
before=$(sigof); perl -e '$t = time - 7; utime $t, $t, @ARGV' "$STATE/dsh10001.txt"
check "the change signature sees a file being touched" test "$before" != "$(sigof)"
eq "the change signature is stable when nothing changed" "$(sigof)" "$(sigof)"
out=$(LINES=3 COLUMNS=80 bounded sitrep watch --once)
check "a 3-line pane gets at most 3 lines" test "$(printf '%s\n' "$out" | wc -l | tr -d ' ')" -le 3
# short blocks: four sessions of two lines each, one header, so nine lines in all
rm -rf "$STATE"; mkdir -p "$STATE"
for n in 1 2 3 4; do printf 'Objective: s%s\n' "$n" > "$STATE/shrt000$n.txt"; printf '/x/d%s\n' "$n" > "$STATE/shrt000$n.cwd"; done
eq "a frame that exactly fits has no footer" 0 "$(LINES=9 COLUMNS=80 bounded sitrep watch --once | grep -c 'more$')"
out=$(LINES=7 COLUMNS=80 bounded sitrep watch --once)
eq "one line short: footer counts the dropped sessions" "+2 more" "$(printf '%s\n' "$out" | tail -1)"
eq "one line short: the footer keeps its own line" 6 "$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
# the signature must see an append that leaves the mtime alone
printf 'a\n' > "$STATE/shrt0001.queue"; perl -e 'utime 1700000000, 1700000000, @ARGV' "$STATE/shrt0001.queue"
before=$(sigof); printf 'b\n' >> "$STATE/shrt0001.queue"; perl -e 'utime 1700000000, 1700000000, @ARGV' "$STATE/shrt0001.queue"
check "the change signature sees a size-only change" test "$before" != "$(sigof)"
eq "the change signature is empty with no state files" "" "$(rm -rf "$STATE/shrt"*; sigof)"
# the live loop, under a real pseudo-terminal
pty() { if script -qc true /dev/null >/dev/null 2>&1; then script -qc "$1" /dev/null; else script -q /dev/null bash -c "$1"; fi; }
if command -v script >/dev/null 2>&1; then
  mk dsh20001 live 10
  out=$( ( sleep 3; printf q ) | TERM=xterm bounded bash -c "$(declare -f pty); pty 'LINES=12 COLUMNS=70 sitrep watch 1; echo \"exit=\$?\"'" 2>&1 )
  check "the live loop draws a frame" has "$out" "Objective: obj dsh20001"
  check "the live loop leaves on q with status 0" has "$out" "exit=0"
  check "the live loop restores the cursor and line wrap on exit" has "$out" $'\033[?7h\033[?25h'
fi
rm -rf "$STATE"; mkdir -p "$STATE"

# ---- queue lists in watch and ls -v
rm -rf "$STATE"; mkdir -p "$STATE"
mk lst00001 lister 60; for i in 1 2 3 4 5 6 7; do sitrep add lst "item $i" >/dev/null; done
frame() { LINES=60 COLUMNS=90 bounded sitrep watch --once "$@"; }
out=$(frame)
check "watch lists the first five items" has "$out" "Queue:     1. item 1"
check "watch lists each item on its own line" has "$out" "               5. item 5"
check "watch says how many more there are" has "$out" "               +2 more"
check "the header shows the count alone" has "$out" "7 items"
check "the header drops the next item" hasnt "$out" "next:"
check "an item past the limit is not listed" hasnt "$out" "6. item 6"
check "-n raises the limit" has "$(frame -n 7)" "7. item 7"
check "-n 7 leaves no more-line" hasnt "$(frame -n 7)" "more"
out=$(frame -n 0)
check "-n 0 is the compact form, with next in the header" has "$out" "7 items, next: item 1"
check "-n 0 lists nothing" hasnt "$out" "Queue:"
check "ls -v lists items too" has "$(sitrep ls -v)" "Queue:     1. item 1"
check "-n implies -v on ls" has "$(sitrep ls -n 2)" "Progress:"
check "ls without -v is unchanged" has "$(sitrep ls)" "7 items, next: item 1"
eq "a bad -n fails" 1 "$(rc sitrep ls -n x)"
eq "a missing -n value fails" 1 "$(rc sitrep ls -n)"
printf 'SITREP_QUEUE_ITEMS=2\n' > "$STATE/sitrep.conf"
check "the default comes from sitrep.conf" has "$(frame)" "               +5 more"
printf 'SITREP_QUEUE_ITEMS="3"\n' > "$STATE/sitrep.conf"
check "a quoted value in sitrep.conf works" has "$(frame)" "               +4 more"
check "the environment beats sitrep.conf" has "$(SITREP_QUEUE_ITEMS=4 frame)" "               +3 more"
check "-n beats the environment" has "$(SITREP_QUEUE_ITEMS=4 frame -n 6)" "               +1 more"
printf 'SITREP_QUEUE_ITEMS=lots\nSITREP_HUD_CMD=echo hi\n' > "$STATE/sitrep.conf"
check "a bad value in sitrep.conf falls back to five" has "$(frame)" "               +2 more"
rm -f "$STATE/sitrep.conf"
for i in 1 2 3 4 5 6 7; do sitrep add inbox "in $i" >/dev/null; done
out=$(frame)
check "the inbox is listed with the same limit" has "$out" "    5. in 5"
check "the inbox says how many more" has "$out" "    +2 more"
eq "the inbox comes first" "Inbox: 7 items" "$(printf '%s\n' "$out" | head -1)"
check "-n applies to the inbox too" hasnt "$(frame -n 2)" "3. in 3"
printf 'esc\033[31mx\n' >> "$STATE/lst00001.queue"
check "listed items are stripped" hasnt "$(frame -n 9)" $'\033'
out=$(LINES=12 COLUMNS=90 bounded sitrep watch --once)
check "a long queue still fits the pane" test "$(printf '%s\n' "$out" | wc -l | tr -d ' ')" -le 12
rm -rf "$STATE"; mkdir -p "$STATE"

# ---- session state: hook events, the running filter, the STATE column
rm -rf "$STATE"; mkdir -p "$STATE"
sword() { cat "$STATE/.$1.state" 2>/dev/null; }
age() { perl -e '$t = time - shift; utime $t, $t, @ARGV' "$1" "$2"; }   # seconds ago, file
ev stt00001 UserPromptSubmit "do the thing" | hook
eq "a prompt that goes to Claude sets working" working "$(sword stt00001)"
ev stt00001 Stop | hook
eq "Stop sets waiting, even with no transcript" waiting "$(sword stt00001)"
out=$(ev stt00001 UserPromptSubmit "+ later" | hook)
eq "a queued prompt does not set working" waiting "$(sword stt00001)"
ev stt00001 UserPromptSubmit "+pin A goal" | hook >/dev/null
ev stt00001 UserPromptSubmit "+inbox thing" | hook >/dev/null
eq "+pin and +inbox do not set working" waiting "$(sword stt00001)"
ev stt00001 UserPromptSubmit "+take 9" | hook >/dev/null
eq "a +take that falls through to Claude sets working" working "$(sword stt00001)"
rm -f "$STATE/inbox.queue"
out=$(ev stt00001 SessionEnd | hook); rcode=$(rc bash -c "$(declare -f ev); ev stt00001 SessionEnd | bash '$R/hooks/sitrep-hook.sh'")
eq "SessionEnd sets ended" ended "$(sword stt00001)"
eq "SessionEnd prints nothing" "" "$out"
eq "SessionEnd exits 0" 0 "$rcode"
ev stt00001 SessionStart | hook
eq "SessionStart un-ends a session that already has an Objective" waiting "$(sword stt00001)"
eq "the state file is owner-only" 600 "$(mode "$STATE/.stt00001.state")"
ev inbox SessionEnd | hook
check "a session id of inbox gets no state" test ! -e "$STATE/.inbox.state"
rm -f "$STATE/.inbox.state"
check "a failing state write never fails the hook" bash -c "chmod 500 '$STATE'; $(declare -f ev); ev stt00001 Stop | bash '$R/hooks/sitrep-hook.sh'; r=\$?; chmod 700 '$STATE'; exit \$r"

rm -rf "$STATE"; mkdir -p "$STATE"
mk stt10001 run 100; mk stt10002 fin 100; mk stt10003 old 100; mk stt10004 nostate 100; mk stt10005 junk 100
printf 'waiting\n' > "$STATE/.stt10001.state"; age 180 "$STATE/.stt10001.state"
printf 'ended\n' > "$STATE/.stt10002.state"
printf 'working\n' > "$STATE/.stt10003.state"; age 30 "$STATE/.stt10003.state"
printf 'rm -rf /\033[31m\n' > "$STATE/.stt10005.state"
mkdir -p "$HOME/.claude/projects/p"
printf '{}\n' > "$HOME/.claude/projects/p/stt10003.jsonl"; age 1500 "$HOME/.claude/projects/p/stt10003.jsonl"
out=$(bounded sitrep ls)
check "the header has a STATE column" has "$out" "STATE"
check "a waiting session shows its age" has "$out" "waiting 3m"
check "an ended session is hidden by default" hasnt "$out" "obj stt10002"
check "the hidden count points at --all" has "$out" "+1 ended (--all)"
check "a working session with a quiet transcript shows idle?" has "$out" "idle? 25m"
check "a session with no state file is listed, with a dash" has "$out" "nostate"
check "an unknown state word is shown as a dash" hasnt "$out" "rm -rf"
check "--all shows the ended session" has "$(bounded sitrep ls --all)" "ended"
check "-a is the same flag" has "$(bounded sitrep ls -a)" "ended"
check "--all leaves no ended footer" hasnt "$(bounded sitrep ls --all)" "(--all)"
check "watch hides ended sessions too" hasnt "$(LINES=40 COLUMNS=90 bounded sitrep watch --once)" "Objective: obj stt10002"
check "watch --all shows them" has "$(LINES=40 COLUMNS=90 bounded sitrep watch --once --all)" "Objective: obj stt10002"
printf '{}\n' > "$HOME/.claude/projects/p/stt10003.jsonl"; age 420 "$STATE/.stt10003.state"
check "a working session with a live transcript stays working" has "$(bounded sitrep ls)" "working 7m"
age 5 "$STATE/.stt10003.state"
check "a short age reads <1m" has "$(bounded sitrep ls)" "working <1m"
eq "the newest state change is listed first" stt1 "$(bounded sitrep ls | sed -n 2p | cut -c1-4)"
check "ls <id> shows the state of an ended session" has "$(sitrep ls stt10002)" "ended"
fn=$(sed -n '/^sig()/,/^}/p' "$R/bin/sitrep")
before=$(bash -c "dir='$STATE'; $fn; sig"); age 1 "$STATE/.stt10001.state"
check "the change signature sees a state change" test "$before" != "$(bash -c "dir='$STATE'; $fn; sig")"
rm -f "$HOME/.claude/projects/p/stt10003.jsonl"
rm -rf "$STATE"; mkdir -p "$STATE"
for n in 1 2 3 4 5; do mk "ftr0000$n" "d$n" 100; printf 'waiting\n' > "$STATE/.ftr0000$n.state"; done; printf 'ended\n' > "$STATE/.ftr00005.state"
for r in 6 10 14 17 18 22; do
  out=$(LINES=$r COLUMNS=80 bounded sitrep watch --once)
  eq "the ended footer stays visible in a $r-line pane" "+1 ended (--all)" "$(printf '%s\n' "$out" | tail -1)"
  check "a $r-line pane gets at most $r lines" test "$(printf '%s\n' "$out" | wc -l | tr -d ' ')" -le "$r"
done
rm -rf "$STATE"; mkdir -p "$STATE"
jq -nc '{session_id:"cmp00001",hook_event_name:"SessionStart",cwd:"/tmp/proj",source:"compact"}' > "$T/compact.json"
jq -nc '{session_id:"cmp00001",hook_event_name:"SessionStart",cwd:"/tmp/proj",source:"resume"}' > "$T/resume.json"
ev cmp00001 UserPromptSubmit "long job" | hook
hook < "$T/compact.json"
eq "a compaction mid-turn leaves working alone" working "$(sword cmp00001)"
hook < "$T/resume.json"
eq "a resume sets waiting" waiting "$(sword cmp00001)"
rm -rf "$STATE"; mkdir -p "$STATE"
eq "with nothing running, watch says so" "No running sessions." "$(LINES=10 COLUMNS=60 bounded sitrep watch --once)"

# ---- live registry: which sessions are running, and prune
rm -rf "$STATE"; mkdir -p "$STATE" "$HOME/.claude/sessions"; rm -f "$HOME/.claude/sessions"/*.json
sleep 300 & p1=$!; sleep 300 & p2=$!; sleep 300 & p3=$!; sleep 300 & p4=$!; sleep 0 & pd=$!; wait "$pd"
lstart() { LC_ALL=C TZ=UTC ps -o lstart= -p "$1" | tr -s ' ' | sed 's/^ //; s/ $//'; }
reg() { # id pid procStart [pidDomain]
  jq -nc --arg s "$1" --argjson p "$2" --arg t "$3" --arg d "${4:-}" '{pid:$p,sessionId:$s,procStart:$t,kind:"interactive",entrypoint:"cli"} + (if $d == "" then {} else {pidDomain:$d} end)' > "$HOME/.claude/sessions/$2.json"
}
st() { printf '%s\n' "$2" > "$STATE/.$1.state"; age "$3" "$STATE/.$1.state"; }   # id word seconds-ago
mk liv00001 dalive 500; st liv00001 waiting 500
mk dea00002 ddead 500;  st dea00002 waiting 500
mk new00003 dfresh 30;  st new00003 waiting 30
mk end00004 dstate 30;  st end00004 ended 30
mk rev00005 drevive 500; st rev00005 ended 500
mk frn00006 dforeign 500; st frn00006 waiting 500
mk reu00007 dreuse 500; st reu00007 waiting 500
reg liv00001 "$p1" "$(lstart "$p1")"
reg rev00005 "$p2" "$(lstart "$p2")"
reg frn00006 "$pd" "Mon Jan  1 00:00:00 2001" "linux:other:ns"
reg reu00007 "$p3" "Mon Jan  1 00:00:00 2001"
reg '../x' "$p4" "$(lstart "$p4")"   # a bad id from the registry is ignored
out=$(bounded sitrep ls)
check "a session with a live process is listed" has "$out" "dalive"
check "a session that is not registered and quiet is hidden" hasnt "$out" "ddead"
check "a new unregistered session gets two minutes of grace" has "$out" "dfresh"
check "a session that said ended is hidden" hasnt "$out" "dstate"
check "the registry wins over a stale ended state" has "$out" "drevive"
check "a live session with a stale ended state reads running" has "$out" "running"
check "a registry entry from another pid space counts as alive" has "$out" "dforeign"
check "a reused pid does not keep a session alive" hasnt "$out" "dreuse"
check "the hidden sessions are counted" has "$out" "+3 ended (--all)"
check "--all shows a dead session as ended" has "$(bounded sitrep ls --all)" "ended"
check "--all lists the hidden session" has "$(bounded sitrep ls --all)" "ddead"
check "watch follows the same rule" hasnt "$(LINES=60 COLUMNS=90 bounded sitrep watch --once)" "ddead"
sitrep add dea 'orphaned task' >/dev/null; sitrep add liv 'keep me' >/dev/null
check "items stranded in a dead session are flagged" has "$(bounded sitrep ls)" "1 items stranded in ended sessions (sitrep prune)"
check "the stranded line keeps its row in a tight pane" has "$(LINES=6 COLUMNS=90 bounded sitrep watch --once)" "stranded"
before=$(cat "$STATE/dea00002.queue")
out=$(sitrep prune --dry-run)
check "--dry-run says what it would mark" has "$out" "would mark dea0 (ddead) ended"
check "--dry-run says what it would ask" has "$out" "would ask: move 1 item(s) from dea0"
eq "--dry-run changes nothing" "waiting" "$(sword dea00002)"
out=$(sitrep prune 2>&1 </dev/null)
check "prune without a terminal or -y leaves the items" has "$out" "left on dea0"
eq "that prune still marked the session ended" ended "$(sword dea00002)"
eq "that prune left the queue alone" "$before" "$(cat "$STATE/dea00002.queue")"
eq "prune never touches a session with a live process" waiting "$(sword liv00001)"
eq "prune never touches a session inside the grace period" waiting "$(sword new00003)"
eq "prune never touches a live session's queue" "keep me" "$(cat "$STATE/liv00001.queue")"
out=$(sitrep prune -y)
check "prune -y moves the items to the inbox" has "$out" "1 item(s) moved to the inbox"
eq "the inbox has the item" "orphaned task" "$(cat "$STATE/inbox.queue")"
eq "the dead session's queue is empty" 0 "$(grep -c . "$STATE/dea00002.queue")"
check "the stranded line is gone" hasnt "$(bounded sitrep ls)" "stranded"
eq "the state file is owner-only" 600 "$(mode "$STATE/.dea00002.state")"
rm -f "$HOME/.claude/sessions"/*.json
st dea00002 waiting 500
eq "prune refuses when the registry cannot be read" 1 "$(rc sitrep prune)"
check "without a registry the listing falls back to the hook states" has "$(bounded sitrep ls)" "ddead"
check "the fallback still hides a session that said ended" hasnt "$(bounded sitrep ls)" "dstate"
kill "$p1" "$p2" "$p3" "$p4" 2>/dev/null; wait "$p1" "$p2" "$p3" "$p4" 2>/dev/null
rm -f "$STATE/inbox.queue"; rm -rf "$STATE"; mkdir -p "$STATE"

# ---- wrapping in watch
mkwide() { printf 'Objective: %s\nProgress:  %s\nNow:       %s\n' "$2" "$3" "$4" > "$STATE/$1.txt"; printf '/x/wrap\n' > "$STATE/$1.cwd"; }
mkwide wrp00001 "short objective" "$(printf 'alpha beta gamma delta %.0s' 1 2 3 4 5 6 7 8)" "now text"
out=$(LINES=60 COLUMNS=40 bounded sitrep watch --once)
check "a long field wraps onto more lines" has "$out" "               alpha"
eq "a wrapped line is indented under the text" 0 "$(printf '%s\n' "$out" | grep -c '^ \{1,14\}[a-z]')"
eq "no line is wider than the pane" 0 "$(printf '%s\n' "$out" | perl -CSD -ne 'chomp; print "x\n" if length > 40' | wc -l | tr -d ' ')"
eq "a field is cut to three lines, with an ellipsis" 4 "$(printf '%s\n' "$out" | sed -n '/Progress:/,/Now:/p' | wc -l | tr -d ' ')"
check "the cut field ends in an ellipsis" has "$(printf '%s\n' "$out" | sed -n '/Progress:/,/Now:/p')" "…"
out=$(LINES=60 COLUMNS=40 bounded sitrep watch --once --wrap 1)
eq "--wrap 1 keeps one line per field" 3 "$(printf '%s\n' "$out" | sed -n '/Objective:/,/Now:/p' | wc -l | tr -d ' ')"
eq "--wrap 0 is refused" 1 "$(rc sitrep watch --once --wrap 0)"
eq "--wrap needs a number" 1 "$(rc sitrep watch --once --wrap x)"
check "plain ls -v does not wrap" has "$(sitrep ls -v | grep Progress)" "gamma delta alpha"
mkwide wrp00002 "$(printf '日本語のテキスト%.0s' 1 2 3 4)" "ok" "ok"
out=$(LINES=60 COLUMNS=30 bounded sitrep watch --once)
eq "wide characters count as two columns" 0 "$(printf '%s\n' "$out" | perl -CSD -ne 'chomp; $w = 0; $w += (/[\x{2E80}-\x{A4CF}]/ ? 2 : 1) for split //; print "x\n" if $w > 30' | wc -l | tr -d ' ')"
mkwide wrp00003 "$(printf 'x%.0s' $(seq 1 120))" "ok" "ok"
out=$(LINES=60 COLUMNS=40 bounded sitrep watch --once)
eq "a word longer than the pane is broken" 0 "$(printf '%s\n' "$out" | perl -CSD -ne 'chomp; print "x\n" if length > 40' | wc -l | tr -d ' ')"
rm -f "$STATE"/wrp*; printf 'Objective: q\nProgress:  p\nNow:       n\n' > "$STATE/qwr00001.txt"; printf '/x/qw\n' > "$STATE/qwr00001.cwd"
sitrep add qwr "$(printf 'item words go on and on %.0s' 1 2 3 4 5 6)" >/dev/null
out=$(LINES=60 COLUMNS=44 bounded sitrep watch --once)
eq "a queue item wraps to at most two lines" 2 "$(printf '%s\n' "$out" | sed -n '/Queue:/,$p' | wc -l | tr -d ' ')"
check "a queue item wraps with a hanging indent" has "$out" "                  "
check "a cut queue item ends in an ellipsis" has "$out" "…"
rm -rf "$STATE"; mkdir -p "$STATE"

# ---- highlight recent activity in watch
rm -rf "$STATE"; mkdir -p "$STATE"
mk hot00001 hot 10; st hot00001 working 10
mk cld00002 cold 500; st cld00002 waiting 500
hlrun() { ( sleep 3; printf q ) | TERM=xterm bounded bash -c "$(declare -f pty); pty '$1 LINES=30 COLUMNS=60 sitrep watch $2 100; echo \"exit=\$?\"'" 2>&1; }
if command -v script >/dev/null 2>&1; then
  out=$(hlrun "" "")
  eq "only the recently active session is highlighted, on each of its lines" 4 "$(printf '%s\n' "$out" | grep -cF $'\033[48;5;238m')"
  check "the highlight fills the rest of the row and is reset" has "$out" $'\033[K\033[0m'
  eq "--highlight 0 turns it off" 0 "$(hlrun "" "--highlight 0" | grep -cF $'\033[48;5;238m')"
  eq "NO_COLOR turns it off" 0 "$(hlrun "NO_COLOR=1" "" | grep -cF $'\033[48;5;238m')"
  eq "a wider window highlights the older session too" 8 "$(hlrun "" "--highlight 1000" | grep -cF $'\033[48;5;238m')"
  printf 'SITREP_HIGHLIGHT_SGR=41\n' > "$STATE/sitrep.conf"
  check "the colour comes from sitrep.conf" has "$(hlrun "" "")" $'\033[41m'
  # shellcheck disable=SC2016  # the value must stay literal: it is the unsafe input under test
  printf 'SITREP_HIGHLIGHT_SGR=1m;$(x)\n' > "$STATE/sitrep.conf"
  eq "a bad colour setting falls back to the default" 4 "$(hlrun "" "" | grep -cF $'\033[48;5;238m')"
  printf 'SITREP_HIGHLIGHT_SECONDS=1000\n' > "$STATE/sitrep.conf"
  eq "the window comes from sitrep.conf" 8 "$(hlrun "" "" | grep -cF $'\033[48;5;238m')"
  rm -f "$STATE/sitrep.conf"
fi
check "--once prints no escape codes" hasnt "$(LINES=30 COLUMNS=60 bounded sitrep watch --once)" $'\033'
check "a pipe prints no escape codes" hasnt "$(LINES=30 COLUMNS=60 bounded sitrep watch)" $'\033'
eq "--highlight needs a number" 1 "$(rc sitrep watch --once --highlight x)"
eq "--highlight needs a value" 1 "$(rc sitrep watch --once --highlight)"
rm -rf "$STATE"; mkdir -p "$STATE"

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
H26="$T/home26"; fresh "$H26" '{}'; mkdir -p "$H26/.claude/session-notes"
echo "Objective: x" > "$H26/.claude/session-notes/abcd0001.txt"; echo waiting > "$H26/.claude/session-notes/.abcd0001.state"
instc "$H26" "$R" install.sh >/dev/null 2>&1
check "install migrates a state file from the old folder" test -f "$H26/.claude/sitrep/.abcd0001.state" -a ! -e "$H26/.claude/session-notes"
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

# sitrep.conf keeps the user's own settings
H21="$T/home21"; fresh "$H21" '{"statusLine":{"type":"command","command":"echo hud"}}'; mkdir -p "$H21/.claude/sitrep"
printf 'SITREP_QUEUE_ITEMS=3\n' > "$H21/.claude/sitrep/sitrep.conf"
instc "$H21" "$R" install.sh >/dev/null 2>&1
check "install keeps a setting in sitrep.conf" grep -q '^SITREP_QUEUE_ITEMS=3$' "$H21/.claude/sitrep/sitrep.conf"
check "install saves the upstream beside it" grep -q '^SITREP_HUD_CMD=' "$H21/.claude/sitrep/sitrep.conf"
instc "$H21" "$R" install.sh >/dev/null 2>&1
eq "a second install keeps one upstream line" 1 "$(grep -c '^SITREP_HUD_CMD=' "$H21/.claude/sitrep/sitrep.conf")"
instc "$H21" "$R" uninstall.sh >/dev/null 2>&1
eq "uninstall keeps the user's setting" "SITREP_QUEUE_ITEMS=3" "$(cat "$H21/.claude/sitrep/sitrep.conf")"
H22="$T/home22"; fresh "$H22" '{}'; mkdir -p "$H22/.claude/sitrep"; printf 'SITREP_QUEUE_ITEMS=8\nSITREP_HUD_CMD=stale\n' > "$H22/.claude/sitrep/sitrep.conf"
instc "$H22" "$R" install.sh >/dev/null 2>&1
eq "with no status line, install drops only the upstream line" "SITREP_QUEUE_ITEMS=8" "$(cat "$H22/.claude/sitrep/sitrep.conf")"

# the SessionEnd hook: fresh install, upgrade from three events, a user's own hook, uninstall
H23="$T/home23"; fresh "$H23" '{}'
out=$(instc "$H23" "$R" install.sh 2>&1)
eq "install registers all four events" 4 "$(jq '[.hooks[]|length]|add' "$H23/.claude/settings.json")"
check "install says it registered SessionEnd" has "$out" "Registered the SessionEnd hook"
check "a second install does not say it again" hasnt "$(instc "$H23" "$R" install.sh 2>&1)" "Registered the SessionEnd hook"
eq "a second install adds no duplicate" 1 "$(jq '.hooks.SessionEnd|length' "$H23/.claude/settings.json")"
H24="$T/home24"; fresh "$H24" "$(jq -nc --arg c "bash $R/hooks/sitrep-hook.sh" '{hooks:{SessionStart:[{hooks:[{type:"command",command:$c}]}],UserPromptSubmit:[{hooks:[{type:"command",command:$c}]}],Stop:[{hooks:[{type:"command",command:$c}]}],SessionEnd:[{hooks:[{type:"command",command:"my-end.sh",timeout:5}]}]}}')"
out=$(instc "$H24" "$R" install.sh 2>&1)
check "an upgrade adds the SessionEnd hook" has "$out" "Registered the SessionEnd hook"
eq "an upgrade keeps one hook per old event" 3 "$(jq '[.hooks.SessionStart,.hooks.UserPromptSubmit,.hooks.Stop|length]|add' "$H24/.claude/settings.json")"
eq "an upgrade keeps the user's SessionEnd hook" 1 "$(jq '[.hooks.SessionEnd[].hooks[]|select(.command=="my-end.sh" and .timeout==5)]|length' "$H24/.claude/settings.json")"
eq "an upgrade adds ours beside it" 1 "$(jq '[.hooks.SessionEnd[].hooks[]|select(.command|contains("sitrep-hook.sh"))]|length' "$H24/.claude/settings.json")"
instc "$H24" "$R" uninstall.sh >/dev/null 2>&1
eq "uninstall leaves only the user's SessionEnd hook" '[{"hooks":[{"type":"command","command":"my-end.sh","timeout":5}]}]' "$(jq -c '.hooks.SessionEnd' "$H24/.claude/settings.json")"
eq "uninstall leaves no empty event arrays" 1 "$(jq '.hooks|length' "$H24/.claude/settings.json")"
instc "$H23" "$R" uninstall.sh >/dev/null 2>&1
check "uninstall removes SessionEnd when it was only ours" jq -e 'has("hooks")|not' "$H23/.claude/settings.json"

# an install must finish when sitrep.conf cannot be written
H25="$T/home25"; fresh "$H25" '{"statusLine":{"type":"command","command":"echo hud"}}'; mkdir -p "$H25/.claude/sitrep/sitrep.conf"
out=$(instc "$H25" "$R" install.sh 2>&1); rcode=$?
eq "install exits 0 when sitrep.conf cannot be written" 0 "$rcode"
check "install says it could not save the upstream" has "$out" "Could not save the upstream status line"
check "install still links sitrep after that" test -L "$H25/bin/sitrep"
check "install still wrote the hooks after that" jq -e '.hooks.Stop|length==1' "$H25/.claude/settings.json"
eq "watch refuses options after the interval" 1 "$(rc sitrep watch 5 -n 3)"
eq "watch accepts options before the interval" 0 "$(rc sitrep watch -n 3 5)"

echo "passed: $pass, failed: $fail"
[ "$fail" -eq 0 ]
