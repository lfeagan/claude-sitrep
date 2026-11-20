#!/bin/bash
# Hook for SessionStart, UserPromptSubmit and Stop. Maintains ~/.claude/session-notes/<session_id>.txt
# (three lines: Objective / Progress / Now). Haiku summarizer runs in the background on Stop.
[ -n "$CLAUDE_NOTES_RUNNING" ] && exit 0   # recursion guard: the summarizer's own claude -p fires hooks
input=$(cat)
sid=$(printf '%s' "$input" | jq -r '.session_id // empty')
[ -z "$sid" ] && exit 0
event=$(printf '%s' "$input" | jq -r '.hook_event_name // empty')
tp=$(printf '%s' "$input" | jq -r '.transcript_path // empty')
dir="$HOME/.claude/session-notes"; mkdir -p "$dir"
f="$dir/$sid.txt"

get() { [ -f "$f" ] && sed -n "s/^$1: //p" "$f" | head -1; }
write() { # objective progress now
  printf 'Objective: %s\nProgress:  %s\nNow:       %s\n' "$1" "$2" "$3" > "$f.tmp" && mv "$f.tmp" "$f"
}
trunc() { printf '%s' "$1" | tr '\n' ' ' | cut -c1-${2:-140}; }

printf '%s' "$input" | jq -r '.cwd // empty' > "$dir/$sid.cwd"
qfile="$dir/$sid.queue"
obj=$(get Objective); prog=$(get Progress); now=$(get Now)
# A placeholder or command wrapper (e.g. /clear) is not a real Objective: treat it as unset so it gets replaced.
case "$obj" in "-"|"<"*|"&lt;"*) obj="" ;; esac

first_prompt() { # first real user message in the transcript (skips command/system wrappers)
  [ -f "$tp" ] && jq -r 'select(.type=="user") | .message.content | if type=="string" then . else map(select(.type=="text")|.text)|join(" ") end' "$tp" 2>/dev/null | grep -v -e '^<' -e '^&lt;' -e '^$' | head -1
}
summarize() { # background Haiku call: rewrites Progress and Now
  (
    export CLAUDE_NOTES_RUNNING=1
    ctx=$(tail -n 40 "$tp" | jq -r 'select(.type=="user" or .type=="assistant") | "\(.type): " + ((.message.content | if type=="string" then . else map(select(.type=="text") | .text) | join(" ") end) // "")' 2>/dev/null | cut -c1-600 | tail -c 6000)
    out=$(printf 'Objective so far: %s\n\nRecent transcript:\n%s\n\nReply with exactly two lines and nothing else:\nProgress: <what has been accomplished so far, max 120 chars>\nNow: <what is happening or awaiting the user right now, max 120 chars>' "$obj" "$ctx" \
      | claude -p --model haiku --no-session-persistence --disable-slash-commands --tools "" --max-budget-usd 0.05 2>/dev/null)
    p=$(printf '%s' "$out" | sed -n 's/^Progress: *//p' | head -1)
    n=$(printf '%s' "$out" | sed -n 's/^Now: *//p' | head -1)
    [ -n "$p" ] && [ -n "$n" ] && write "$(get Objective)" "$(trunc "$p" 140)" "$(trunc "$n" 140)"
  ) >/dev/null 2>&1 &
}

if [ "$event" = "UserPromptSubmit" ]; then
  prompt=$(printf '%s' "$input" | jq -r '.prompt // empty')
  case "$prompt" in
    "+goal "*) # pin the objective; the summarizer never overwrites it
      obj=$(trunc "${prompt#+goal }" 160)
      write "$obj" "${prog:--}" "${now:--}"
      jq -n --arg r "Objective pinned: $obj" '{decision:"block",reason:$r}'; exit 0 ;;
    "+ "*) # queue instead of sending to Claude
      item=${prompt#+ }; printf '%s\n' "$item" >> "$qfile"
      [ -f "$f" ] || write "-" "-" "-"
      jq -n --arg r "Queued ($(grep -c . "$qfile")): $item" '{decision:"block",reason:$r}'; exit 0 ;;
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
  # debounce: skip if summarized in the last 45s
  stamp="$dir/.$sid.stamp"
  if [ -f "$stamp" ] && [ $(( $(date +%s) - $(stat -f %m "$stamp") )) -lt 45 ]; then exit 0; fi
  touch "$stamp"
  summarize
fi
exit 0
