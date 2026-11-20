#!/bin/bash
# Status line wrapper: runs an optional upstream status line (SITREP_HUD_CMD, e.g. Claude HUD),
# then appends the per-session Objective / Progress / Now block and the queue line.
input=$(cat)
conf="$HOME/.claude/sitrep.conf"
# shellcheck source=/dev/null
[ -f "$conf" ] && . "$conf"
[ -n "$SITREP_HUD_CMD" ] && printf '%s' "$input" | sh -c "$SITREP_HUD_CMD"
sid=$(printf '%s' "$input" | jq -r '.session_id // empty')
dir="$HOME/.claude/session-notes"; f="$dir/$sid.txt"
case "$sid" in *[!A-Za-z0-9_-]*) sid="" ;; esac
# Our own block is untrusted text: strip control characters (keeps tab and newline).
clean() { LC_ALL=C tr -d '\000-\010\013-\037\177'; }
if [ -n "$sid" ]; then
  if [ -f "$f" ]; then clean < "$f"; else echo "Objective: (set on your first prompt)"; fi
  qf="$dir/$sid.queue"; n=0; next=""
  [ -s "$qf" ] && { n=$(grep -c . "$qf"); next=$(head -1 "$qf" | cut -c1-90 | clean); }
  printf 'Queue (%s) [%s]%s\n' "$n" "${sid:0:4}" "${next:+  next: $next}"
fi
exit 0
