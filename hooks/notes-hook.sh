#!/bin/bash
# Hook for SessionStart, UserPromptSubmit and Stop. Maintains ~/.claude/sitrep/<session_id>.txt
# (three lines: Objective / Progress / Now). A Haiku summarizer runs in the background on Stop.
[ -n "$CLAUDE_NOTES_RUNNING" ] && exit 0   # recursion guard: the summarizer's own claude -p fires hooks
umask 077   # notes hold prompt text: owner-only
input=$(cat)
sid=$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)
[ -z "$sid" ] && exit 0
case "$sid" in *[!A-Za-z0-9_-]*) exit 0 ;; esac   # the id becomes part of a file name
event=$(printf '%s' "$input" | jq -r '.hook_event_name // empty')
tp=$(printf '%s' "$input" | jq -r '.transcript_path // empty')
# Only read transcripts that are regular files under the Claude projects directory.
cfg=${CLAUDE_CONFIG_DIR:-$HOME/.claude}
case "$tp" in
  */../*) tp="" ;;
  "$cfg/projects/"*) { [ -f "$tp" ] && [ ! -L "$tp" ]; } || tp="" ;;
  *) tp="" ;;
esac
dir="$HOME/.claude/sitrep"; [ -d "$dir" ] || mkdir -p "$dir"; chmod 700 "$dir"
f="$dir/$sid.txt"

mtime() { stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null; }
# Prompt and model text is untrusted terminal input. Flatten to one line, drop control (Cc, incl. C1) and
# format (Cf, incl. bidi) characters, and cut to $1 characters.
sane() { N=${1:-140} perl -CSD -0777 -pe 's/[\n\t\p{Zl}\p{Zp}]/ /g; s/[\p{Cc}\p{Cf}]//g; $_=substr($_,0,$ENV{N})' 2>/dev/null; }
trunc() { printf '%s' "$1" | sane "${2:-140}"; }
get() { [ -f "$f" ] && sed -n "s/^$1: *//p" "$f" | head -1; }   # label plus all padding
write() { # objective progress now
  local t; t=$(mktemp "$dir/.w.XXXXXX") || return 1
  if printf 'Objective: %s\nProgress:  %s\nNow:       %s\n' "$1" "$2" "$3" > "$t"; then mv "$t" "$f"; else rm -f "$t"; return 1; fi
}

printf '%s' "$input" | jq -r '.cwd // empty' | sane 300 > "$dir/$sid.cwd"
qfile="$dir/$sid.queue"
obj=$(get Objective); prog=$(get Progress); now=$(get Now)
# A placeholder or command wrapper (e.g. /clear) is not a real Objective: treat it as unset so it gets replaced.
case "$obj" in "-"|*"<command-"*|*"&lt;command-"*|"<"*|"&lt;"*) obj="" ;; esac

short() { [[ $1 != *$'\n'* ]] && [ ${#1} -le 200 ] && [ -n "${1//[[:space:]]/}" ]; }
block() { jq -n --arg r "$1" '{decision:"block",reason:$r}'; }

first_prompt() { # first real user message in the transcript; skips whole messages that are command/system wrappers (start with a tag)
  [ -f "$tp" ] && jq -r 'select(.type=="user") | .message.content | if type=="string" then . else map(select(.type=="text")|.text)|join(" ") end | select(test("^\\s*(<|&lt;)")|not) | select(test("\\S"))' "$tp" 2>/dev/null | sed -e 's/^[[:space:]]*//' -e '/^$/d' | head -1
}
pinned() { [ -e "$dir/.$sid.pinned" ]; }   # set by +goal and `sitrep goal`: the summarizer then leaves Objective alone
summarize() { # background Haiku call: rewrites Progress and Now, and Objective unless pinned
  local since=; { read -r since < "$dir/.$sid.prompt"; } 2>/dev/null
  (
    export CLAUDE_NOTES_RUNNING=1
    work=$(mktemp -d) && cd "$work" || exit 0   # neutral cwd: no project CLAUDE.md, settings or MCP servers
    trap 'rm -rf "$work"' EXIT
    ctx=$(tail -n 40 "$tp" | jq -r 'select(.type=="user" or .type=="assistant") | "\(.type): " + ((.message.content | if type=="string" then . else map(select(.type=="text") | .text) | join(" ") end) // "")' 2>/dev/null | cut -c1-600 | tail -c 6000)
    out=$(printf 'Objective so far: %s\n\nRecent transcript:\n%s\n\nReply with exactly three lines and nothing else:\nObjective: <a short title for the overall goal of this session, max 80 chars; keep the objective so far if it still fits>\nProgress: <what has been accomplished so far, max 120 chars>\nNow: <what is happening or awaiting the user right now, max 120 chars>' "$obj" "$ctx" \
      | perl -e 'alarm shift; exec @ARGV' 60 claude -p --model haiku --no-session-persistence --disable-slash-commands --tools "" \
          --strict-mcp-config --mcp-config '{"mcpServers":{}}' --setting-sources "" --max-budget-usd 0.05 2>/dev/null)
    o=$(printf '%s' "$out" | sed -n 's/^Objective: *//p' | head -1)
    p=$(printf '%s' "$out" | sed -n 's/^Progress: *//p' | head -1)
    n=$(printf '%s' "$out" | sed -n 's/^Now: *//p' | head -1)
    [ -n "$p" ] && [ -n "$n" ] || exit 0
    # A prompt submitted while we were summarizing makes this summary stale: drop it, the next Stop redoes it.
    [ "$(cat "$dir/.$sid.prompt" 2>/dev/null)" = "$since" ] || exit 0
    o=$(trunc "$o" 100); { pinned || [ -z "$o" ]; } && o=$(get Objective)
    write "$o" "$(trunc "$p" 140)" "$(trunc "$n" 140)"
  ) >/dev/null 2>&1 &
}

if [ "$event" = "UserPromptSubmit" ]; then
  prompt=$(printf '%s' "$input" | jq -r '.prompt // empty')
  c=0; { read -r c < "$dir/.$sid.prompt"; } 2>/dev/null
  echo $(( ${c:-0} + 1 )) > "$dir/.$sid.prompt"   # prompt counter, for the stale-summary check
  # +goal and + are handled here only for a short single line. Anything else goes to Claude untouched.
  case "$prompt" in
    "+goal "*) # pin the objective; the summarizer never overwrites it
      text=${prompt#+goal }
      if short "$text"; then
        g=$(trunc "$text" 200)
        if : > "$dir/.$sid.pinned" && write "$g" "${prog:--}" "${now:--}"; then
          block "Objective pinned: $g"; exit 0
        fi
      fi ;;
    "+ "*) # queue instead of sending to Claude
      text=${prompt#+ }
      if short "$text"; then
        item=$(trunc "$text" 200)
        if [ -n "$item" ] && printf '%s\n' "$item" >> "$qfile"; then
          [ -f "$f" ] || write "-" "-" "-"
          block "Queued ($(grep -c . "$qfile")): $item"; exit 0
        fi
      fi ;;
  esac
  [ -z "$obj" ] && obj=$(trunc "$(first_prompt)" 160)
  [ -z "$obj" ] && obj=$(trunc "$prompt" 160)
  write "$obj" "${prog:--}" "$(trunc "$prompt" 140)"
  exit 0
fi

if [ "$event" = "SessionStart" ]; then
  # Resumed or continued sessions already have history: seed the notes from the transcript.
  if [ -z "$obj" ] && [ -f "$tp" ]; then
    obj=$(trunc "$(first_prompt)" 160)
    write "${obj:--}" "-" "(resumed -- summarizing)"
    [ -n "$obj" ] && summarize
  fi
  exit 0
fi

if [ "$event" = "Stop" ] && [ -f "$tp" ]; then
  # Debounce: at most one summary per window. A Stop inside the window schedules one deferred run for the
  # end of the window, so a quick follow-up turn is still summarized and never left stale.
  window=${SITREP_DEBOUNCE:-45}
  stamp="$dir/.$sid.stamp"; pending="$dir/.$sid.pending"
  last=$(mtime "$stamp"); ts=$(date +%s)
  if [ -n "$last" ] && [ $(( ts - last )) -lt "$window" ]; then
    [ -e "$pending" ] && exit 0
    : > "$pending"
    wait_s=$(( window - (ts - last) ))
    (
      sleep "$wait_s"; rm -f "$pending"
      obj=$(get Objective); touch "$stamp"; summarize
    ) >/dev/null 2>&1 &
    exit 0
  fi
  touch "$stamp"
  summarize
fi
exit 0
