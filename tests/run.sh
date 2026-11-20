#!/bin/bash
# Test suite. Needs bash, jq, perl and tar. Everything runs against a throwaway $HOME and a stub `claude`,
# so it never touches real settings, real notes or the API.
R=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
export HOME="$T/home"; mkdir -p "$HOME/.claude" "$T/bin"
cat > "$T/bin/claude" <<'STUB'
#!/bin/bash
cat >/dev/null
echo x >> "$STUB_CALLS"
[ -e "$STUB_SLOW" ] && sleep 2
printf 'Progress: stub progress\nNow: stub now\n'
STUB
chmod +x "$T/bin/claude"
export PATH="$T/bin:$R/bin:$PATH" STUB_CALLS="$T/calls" STUB_SLOW="$T/slow"
NOTES="$HOME/.claude/session-notes"
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

hook() { bash "$R/hooks/notes-hook.sh"; }                                  # reads event JSON on stdin
ev() { jq -nc --arg s "$1" --arg e "$2" --arg p "${3:-}" --arg t "${4:-}" \
         '{session_id:$s,hook_event_name:$e,cwd:"/tmp/proj",prompt:$p,transcript_path:$t}'; }
field() { sed -n "s/^$2: *//p" "$NOTES/$1.txt" | head -1; }
status_in() { jq -nc --arg s "${2:-sx}" '{session_id:$s}' | HOME="$1" bash "$R/statusline/statusline.sh"; }   # home, session
status() { status_in "$HOME" "$1"; }
rc() { "$@" >/dev/null 2>&1; echo $?; }   # exit status of a command, for use inside eq
waitfor() { for _ in $(seq 1 60); do [ "$(field "$1" "$2")" = "$3" ] && return 0; sleep 0.2; done; return 1; }
mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }
reset() { rm -rf "$NOTES" "$HOME/.claude/sitrep.conf" "$STUB_CALLS" "$STUB_SLOW"; }
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
check "escape characters never reach the notes file" nobytes "$NOTES/sess0002.txt" $'\033'
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
mkdir -p "$HOME/.claude/projects/p"; TP="$HOME/.claude/projects/p/transcript.jsonl"
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

# ---- hook: C1 controls and bidi overrides are untrusted too
reset
ev sess0011 UserPromptSubmit "$(printf 'hi \xc2\x9b31mRED\xc2\x9d0;t\xc2\x9c \xe2\x80\xaeX')" | hook
check "C1 CSI removed" nobytes "$NOTES/sess0011.txt" $'\xc2\x9b'
check "C1 OSC removed" nobytes "$NOTES/sess0011.txt" $'\xc2\x9d'
check "C1 ST removed" nobytes "$NOTES/sess0011.txt" $'\xc2\x9c'
check "bidi override removed" nobytes "$NOTES/sess0011.txt" $'\xe2\x80\xae'
eq "ordinary text and UTF-8 survive" "héllo 世界" "$(ev sess0012 UserPromptSubmit 'héllo 世界' | hook; field sess0012 Objective)"
printf 'Objective: a\xc2\x9b31m\xe2\x80\xaeb\nProgress: -\nNow: -\n' > "$NOTES/sess0013.txt"
out=$(status sess0013)
check "status line drops C1 and bidi at print time" hasnt "$out" $'\xc2\x9b'
check "status line drops bidi at print time" hasnt "$out" $'\xe2\x80\xae'
check "q ls drops C1 at print time" hasnt "$(q ls)" $'\xc2\x9b'

# ---- hook: + and +goal never lose a prompt
reset
ev sess0020 UserPromptSubmit "start" | hook
out=$(ev sess0020 UserPromptSubmit "+ $(printf 'x%.0s' $(seq 1 300))" | hook)
eq "long + prompt passes through" "" "$out"
out=$(ev sess0020 UserPromptSubmit $'+ line one\nline two' | hook)
eq "multi-line + prompt passes through" "" "$out"
out=$(ev sess0020 UserPromptSubmit "+goal $(printf 'y%.0s' $(seq 1 300))" | hook)
eq "long +goal prompt passes through" "" "$out"
check "nothing was queued" test ! -s "$NOTES/sess0020.queue"
mkdir "$NOTES/sess0020.queue.d"; rm -f "$NOTES/sess0020.queue"; mkdir "$NOTES/sess0020.queue"
out=$(ev sess0020 UserPromptSubmit "+ important" | hook 2>/dev/null)
eq "unwritable queue passes the prompt through" "" "$out"
rmdir "$NOTES/sess0020.queue" "$NOTES/sess0020.queue.d"

# ---- hook: label padding, blank items, deferred debounce
reset
for i in 1 2 3; do ev sess0040 UserPromptSubmit "p$i" | hook; done
eq "label padding does not grow across prompts" "Progress:  -" "$(grep '^Progress:' "$NOTES/sess0040.txt")"
out=$(ev sess0040 UserPromptSubmit "+ $(printf '\001\002')" | hook)
eq "a + item that cleans to nothing passes through" "" "$out"
check "no blank queue line was written" test ! -s "$NOTES/sess0040.queue"

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
eq "q done on an empty queue fails" 1 "$(rc q "done" abc)"
eq "ambiguous prefix fails" 1 "$(rc q ls ab)"
eq "non-numeric item is rejected" 1 "$(rc q "done" abe '1d')"
eq "path-like session is rejected" 1 "$(rc q ls '../x')"
q goal abe "New goal" >/dev/null
eq "q goal pins the objective" "New goal" "$(field abef0002 Objective)"
q add abe $'tab\there \033[31mred\xc2\x9b31m' >/dev/null
check "q add strips escapes" nobytes "$NOTES/abef0002.queue" $'\033'
check "q add strips C1 controls" nobytes "$NOTES/abef0002.queue" $'\xc2\x9b'
check "q ls lists sessions" has "$(q ls)" "queue:"
printf 'Objective: only\n' > "$NOTES/abcd0001.txt"
q goal abcd "replaced" >/dev/null
eq "q goal works on an objective-only file" "replaced" "$(field abcd0001 Objective)"
q add abcd one >/dev/null; q "done" abcd >/dev/null
check "q ls prints no stray count for an emptied queue" bash -c '! q ls | grep -qx 0'
eq "q add reports a zero count once" "queued on abcd (now 1)" "$(q add abcd two)"
eq "q done past the end of the queue fails" 1 "$(rc q "done" abcd 9)"
eq "q add rejects text that cleans to nothing" 1 "$(rc q add abcd "$(printf '\001')")"

# ---- install / uninstall round trip, in a separate HOME
H2="$T/home2"
fresh "$H2" '{"model":"sonnet","statusLine":{"type":"command","command":"echo old-hud"},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo mine"}]}]}}'
cp "$H2/.claude/settings.json" "$T/original.json"
instc "$H2" "$R" install.sh >/dev/null
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
instc "$H2" "$R" install.sh >/dev/null
eq "second install does not duplicate hooks" 1 "$(jq '[.hooks.Stop[]|select(.hooks[0].command|contains("notes-hook"))]|length' "$S2")"
eq "second install keeps the saved upstream" "$conf1" "$(cat "$H2/.claude/sitrep.conf")"
instc "$H2" "$R" uninstall.sh >/dev/null
eq "uninstall restores settings exactly" "$(jq -S . "$T/original.json")" "$(jq -S . "$S2")"
check "uninstall unlinks q" test ! -e "$H2/bin/q"

# ---- install: clones, output and recovery

# an install from a second clone must not save our own status line as the upstream
H3="$T/home3"; fresh "$H3" '{"statusLine":{"type":"command","command":"echo old-hud"}}'
clone "$T/clone-a"; clone "$T/clone-b"
instc "$H3" "$T/clone-a" install.sh >/dev/null; instc "$H3" "$T/clone-b" install.sh >/dev/null 2>&1
check "upstream stays the original after installs from two clones" bash -c "grep -q 'old-hud' '$H3/.claude/sitrep.conf' && ! grep -q 'statusline.sh' '$H3/.claude/sitrep.conf'"
eq "status line refuses to render itself" "" "$(jq -nc '{session_id:"x"}' | SITREP_ACTIVE=1 bash "$R/statusline/statusline.sh")"

# install output says what it found
H9="$T/home9"; fresh "$H9" '{}'
check "install reports no existing status line" has "$(instc "$H9" "$R" install.sh 2>&1)" "No existing status line"
check "no sitrep.conf is written without an upstream" test ! -e "$H9/.claude/sitrep.conf"
check "block renders without an upstream" has "$(status_in "$H9")" "Queue (0)"
H10="$T/home10"; fresh "$H10" '{"statusLine":{"type":"command","command":"echo hud"}}'
check "install reports the status line it wraps" has "$(instc "$H10" "$R" install.sh 2>&1)" "Wrapping the existing status line (it renders first): echo hud"
eq "wrapped upstream renders first" hud "$(status_in "$H10" | head -1)"
# a HUD setup that overwrites our status line is repaired by re-running install
jq '.statusLine.command="echo hud-again"' "$H10/.claude/settings.json" > "$T/h10.json" && cp "$T/h10.json" "$H10/.claude/settings.json"
instc "$H10" "$R" install.sh >/dev/null 2>&1
eq "re-install after an overwrite wraps the new command" hud-again "$(status_in "$H10" | head -1)"
check "sitrep block is back after the overwrite" has "$(status_in "$H10")" "Queue (0)"

# ---- install: stale config, look-alike status line, failed merge, upstream without a newline
H11="$T/home11"; fresh "$H11" '{}'; printf 'SITREP_HUD_CMD=%q\n' "echo stale" > "$H11/.claude/sitrep.conf"
instc "$H11" "$R" install.sh >/dev/null
out=$(status_in "$H11")
check "a stale upstream is cleared when there is no status line" hasnt "$out" "stale"
H12="$T/home12"; fresh "$H12" '{"statusLine":{"type":"command","command":"/opt/other/statusline/statusline.sh"}}'
instc "$H12" "$R" install.sh >/dev/null 2>&1
check "a foreign statusline.sh is wrapped, not mistaken for ours" grep -q "other/statusline" "$H12/.claude/sitrep.conf"
instc "$H12" "$R" uninstall.sh >/dev/null
eq "uninstall restores the foreign status line" "/opt/other/statusline/statusline.sh" "$(jq -r .statusLine.command "$H12/.claude/settings.json")"
check "uninstall removes the consumed upstream config" test ! -e "$H12/.claude/sitrep.conf"
H13="$T/home13"; fresh "$H13" '{"hooks":5,"statusLine":{"type":"command","command":"echo hud"}}'
out=$(instc "$H13" "$R" install.sh 2>&1)
check "a failed merge writes no upstream config" test ! -e "$H13/.claude/sitrep.conf"
check "a failed merge does not claim to wrap anything" hasnt "$out" "Wrapping"
printf 'SITREP_HUD_CMD=%q\n' "printf hud" > "$HOME/.claude/sitrep.conf"
eq "upstream without a trailing newline does not glue onto our block" "hud" "$(status sess0007 | head -1)"
rm -f "$HOME/.claude/sitrep.conf"

# paths with spaces
H4="$T/home4"; fresh "$H4" '{}'
clone "$T/my dir/sitrep"
instc "$H4" "$T/my dir/sitrep" install.sh >/dev/null
cmd=$(jq -r '.hooks.Stop[-1].hooks[0].command' "$H4/.claude/settings.json")
eq "stored hook command runs from a path with spaces" 0 "$(echo '{}' | HOME="$H4" bash -c "$cmd" >/dev/null 2>&1; echo $?)"
slc=$(jq -r '.statusLine.command' "$H4/.claude/settings.json")
check "stored status line command runs from a path with spaces" has "$(jq -nc '{session_id:"sx"}' | HOME="$H4" bash -c "$slc")" "Queue (0)"

# hooks we do not own: non-command hooks, empty groups, look-alike names
H5="$T/home5"; fresh "$H5" '{"hooks":{"Stop":[{"hooks":[{"type":"prompt","prompt":"x"}]},{"hooks":[]},{"hooks":[{"type":"command","command":"my-notes-hook-other.sh"}]}]},"statusLine":{"type":"command","command":"x","padding":2}}'
cp "$H5/.claude/settings.json" "$T/h5.json"
check "install tolerates non-command hooks and empty groups" instc "$H5" "$R" install.sh
eq "install keeps the user's look-alike hook" 1 "$(jq '[.hooks.Stop[]|select(.hooks[0].command=="my-notes-hook-other.sh")]|length' "$H5/.claude/settings.json")"
eq "install keeps statusLine fields it does not own" 2 "$(jq .statusLine.padding "$H5/.claude/settings.json")"
check "our hook is installed" jq -e '[.hooks.Stop[]|select((.hooks[0].command // "")|contains("notes-hook.sh"))]|length==1' "$H5/.claude/settings.json"
instc "$H5" "$R" uninstall.sh >/dev/null
eq "uninstall restores settings with foreign hooks exactly" "$(jq -S . "$T/h5.json")" "$(jq -S . "$H5/.claude/settings.json")"

# a q we do not own
H6="$T/home6"; fresh "$H6" '{}'; mkdir -p "$H6/bin"; echo "mine" > "$H6/bin/q"
instc "$H6" "$R" install.sh >/dev/null 2>&1
eq "install leaves a foreign q alone" mine "$(cat "$H6/bin/q")"
instc "$H6" "$R" uninstall.sh >/dev/null
eq "uninstall leaves a foreign q alone" mine "$(cat "$H6/bin/q")"

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

echo "passed: $pass, failed: $fail"
[ "$fail" -eq 0 ]
