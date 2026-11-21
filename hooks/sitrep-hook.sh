#!/bin/bash
# Hook for SessionStart, UserPromptSubmit and Stop. Maintains ~/.claude/sitrep/<session_id>.txt
# (three lines: Objective / Progress / Now). A Haiku summarizer runs in the background on Stop.
[ -n "$SITREP_SUMMARIZING" ] && exit 0   # recursion guard: the summarizer's own claude -p fires hooks
umask 077   # state holds prompt text: owner-only
input=$(cat)
sid=$(printf '%s' "$input" | jq -r '.session_id // empty' 2>/dev/null)
[ -z "$sid" ] && exit 0
case "$sid" in *[!A-Za-z0-9_-]*|inbox) exit 0 ;; esac   # the id becomes part of a file name; `inbox` is the queue of no session
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

# Once /sitrep:setup has made the stable link and the status line copy, keep them on the newest installed plugin version, from the
# plugin's own hook. It never creates them (that would undo a removal), never moves from a working version to an older one or to a
# folder outside the plugin cache, and never fails.
refresh_plugin() {
  local root=${CLAUDE_PLUGIN_ROOT:-} cur cv nv t
  case "$root" in /*) ;; *) return 0 ;; esac   # an absolute path, without ..: it becomes code the status line runs
  case "$root" in */../*|*/..) return 0 ;; esac
  [ -f "$root/statusline/statusline.sh" ] || return 0
  cur=$(readlink "$dir/plugin" 2>/dev/null)
  if [ "$cur" != "$root" ]; then
    if [ -f "$cur/statusline/statusline.sh" ]; then
      case "$root" in */plugins/cache/*) ;; *) return 0 ;; esac
      cv=$(jq -r '.version // empty' "$cur/.claude-plugin/plugin.json" 2>/dev/null); nv=$(jq -r '.version // empty' "$root/.claude-plugin/plugin.json" 2>/dev/null)
      if [ -z "$nv" ] || [ "$nv" = "$cv" ] || [ "$(printf '%s\n%s\n' "$cv" "$nv" | sort -V | tail -1)" != "$nv" ]; then return 0; fi
    fi
    t="$dir/.plugin.$$"; rm -f "$t"
    if ! { ln -s "$root" "$t" && perl -e 'rename $ARGV[0], $ARGV[1] or exit 1' "$t" "$dir/plugin"; }; then rm -f "$t"; return 0; fi
  fi
  if [ -f "$dir/statusline.sh" ] && [ ! -L "$dir/statusline.sh" ] && ! cmp -s "$root/statusline/statusline.sh" "$dir/statusline.sh"; then
    if t=$(mktemp "$dir/.statusline.XXXXXX"); then
      if cat "$root/statusline/statusline.sh" > "$t"; then mv "$t" "$dir/statusline.sh"; else rm -f "$t"; fi
    fi
  fi
  return 0
}
[ "$event" = SessionStart ] && [ -L "$dir/plugin" ] && { refresh_plugin; } 2>/dev/null

mtime() { stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null; }
# Prompt and model text is untrusted terminal input. Flatten to one line, drop control (Cc, incl. C1) and
# format (Cf, incl. bidi) characters, and cut to $1 characters.
sane() { N=${1:-140} perl -CSD -0777 -pe 's/[\n\t\p{Zl}\p{Zp}]/ /g; s/[\p{Cc}\p{Cf}]//g; $_=substr($_,0,$ENV{N})' 2>/dev/null; }
trunc() { printf '%s' "$1" | sane "${2:-140}"; }
get() { [ -f "$f" ] && sed -n "s/^$1: *//p" "$f" | head -1; }   # label plus all padding
write() { # objective progress now
  local t; t=$(mktemp "$dir/.sitrep.XXXXXX") || return 1
  if printf 'Objective: %s\nProgress:  %s\nNow:       %s\n' "$1" "$2" "$3" > "$t"; then mv "$t" "$f"; else rm -f "$t"; return 1; fi
}

# Session state, for `sitrep watch`: working, waiting or ended. The file's mtime is when it changed.
setstate() {
  local t; t=$(mktemp "$dir/.sitrep.XXXXXX") || return 0
  if { printf '%s\n' "$1" > "$t" && mv "$t" "$dir/.$sid.state"; } 2>/dev/null; then :; else rm -f "$t"; fi
  return 0   # a hook never fails
}
[ "$event" = "SessionEnd" ] && { setstate ended; exit 0; }

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
pinned() { [ -e "$dir/.$sid.pinned" ]; }   # set by +pin and `sitrep pin`: the summarizer then leaves Objective alone
summarize() { # background Haiku call: rewrites Progress and Now, and Objective unless pinned
  local since=; { read -r since < "$dir/.$sid.count"; } 2>/dev/null
  (
    export SITREP_SUMMARIZING=1
    work=$(mktemp -d) && cd "$work" || exit 0   # neutral cwd: no project CLAUDE.md, settings or MCP servers
    trap 'rm -rf "$work"' EXIT
    ctx=$(tail -n 40 "$tp" | jq -r 'select(.type=="user" or .type=="assistant") | "\(.type): " + ((.message.content | if type=="string" then . else map(select(.type=="text") | .text) | join(" ") end) // "")' 2>/dev/null | cut -c1-600 | tail -c 6000)
    out=$(printf 'Objective so far: %s\n\nRecent transcript:\n%s\n\nReply with exactly three lines and nothing else:\nObjective: <a short title for the overall objective of this session, max 80 chars; keep the objective so far if it still fits>\nProgress: <what has been accomplished so far, max 120 chars>\nNow: <what is happening or awaiting the user right now, max 120 chars>' "$obj" "$ctx" \
      | perl -e 'alarm shift; exec @ARGV' 60 claude -p --model haiku --no-session-persistence --disable-slash-commands --tools "" \
          --strict-mcp-config --mcp-config '{"mcpServers":{}}' --setting-sources "" --max-budget-usd 0.05 2>/dev/null)
    o=$(printf '%s' "$out" | sed -n 's/^Objective: *//p' | head -1)
    p=$(printf '%s' "$out" | sed -n 's/^Progress: *//p' | head -1)
    n=$(printf '%s' "$out" | sed -n 's/^Now: *//p' | head -1)
    [ -n "$p" ] && [ -n "$n" ] || exit 0
    # A prompt submitted while we were summarizing makes this summary stale: drop it, the next Stop redoes it.
    [ "$(cat "$dir/.$sid.count" 2>/dev/null)" = "$since" ] || exit 0
    o=$(trunc "$o" 100); { pinned || [ -z "$o" ]; } && o=$(get Objective)
    write "$o" "$(trunc "$p" 140)" "$(trunc "$n" 140)"
  ) >/dev/null 2>&1 &
}

if [ "$event" = "UserPromptSubmit" ]; then
  prompt=$(printf '%s' "$input" | jq -r '.prompt // empty')
  c=0; { read -r c < "$dir/.$sid.count"; } 2>/dev/null
  echo $(( ${c:-0} + 1 )) > "$dir/.$sid.count"   # prompt count, for the stale-summary check
  # +pin, +inbox, +take and + are handled here only for a short single line. Anything else goes to Claude untouched.
  case "$prompt" in
    "+pin "*) # pin the objective; the summarizer never overwrites it
      text=${prompt#+pin }
      if short "$text"; then
        new_obj=$(trunc "$text" 200)
        if : > "$dir/.$sid.pinned" && write "$new_obj" "${prog:--}" "${now:--}"; then
          block "Objective pinned: $new_obj"; exit 0
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
    "+inbox "*) # queue to the inbox, which belongs to no session
      text=${prompt#+inbox }; text=${text#"${text%%[![:space:]]*}"}   # drop leading spaces
      if short "$text"; then
        item=$(trunc "$text" 200)
        if [ -n "$item" ] && printf '%s\n' "$item" >> "$dir/inbox.queue"; then
          block "Inbox ($(grep -c . "$dir/inbox.queue")): $item"; exit 0
        fi
      fi ;;
    "+take"|"+take "*) # move an inbox item (default 1) into this session's queue; sitrep does the work
      n=${prompt#+take}; n=${n# }; n=${n:-1}
      if [[ $n =~ ^[1-9][0-9]*$ ]]; then
        [ -f "$f" ] || write "-" "-" "-"
        if took=$(bash "$(dirname "$0")/../bin/sitrep" move inbox "$sid" "$n" 2>/dev/null); then
          block "Took from the inbox: $(trunc "${took#moved to *: }" 200)"; exit 0
        fi
      fi ;;
  esac
  setstate working   # only a prompt that goes to Claude starts work; the cases above exit before this
  [ -z "$obj" ] && obj=$(trunc "$(first_prompt)" 160)
  [ -z "$obj" ] && obj=$(trunc "$prompt" 160)
  write "$obj" "${prog:--}" "$(trunc "$prompt" 140)"
  exit 0
fi

if [ "$event" = "SessionStart" ]; then
  # also un-ends a resumed session; a compaction fires SessionStart in the middle of a turn, so it leaves the state alone
  [ "$(printf '%s' "$input" | jq -r '.source // empty' 2>/dev/null)" = compact ] || setstate waiting
  # Resumed or continued sessions already have history: seed the state from the transcript.
  if [ -z "$obj" ] && [ -f "$tp" ]; then
    obj=$(trunc "$(first_prompt)" 160)
    write "${obj:--}" "-" "(resumed -- summarizing)"
    [ -n "$obj" ] && summarize
  fi
  exit 0
fi

[ "$event" = "Stop" ] && setstate waiting
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
