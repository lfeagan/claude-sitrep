#!/bin/bash
# Status line wrapper: runs an optional upstream status line (SITREP_HUD_CMD, e.g. Claude HUD),
# then appends the per-session Objective / Progress / Now block and the queue line.
[ -n "$SITREP_ACTIVE" ] && exit 0   # never render ourselves, even if the upstream command points back here
export SITREP_ACTIVE=1
input=$(cat)
conf="$HOME/.claude/sitrep.conf"
# shellcheck source=/dev/null
[ -f "$conf" ] && . "$conf"
if [ -n "$SITREP_HUD_CMD" ]; then
  up=$(printf '%s' "$input" | sh -c "$SITREP_HUD_CMD")   # the substitution drops trailing newlines; we add exactly one
  [ -n "$up" ] && printf '%s\n' "$up"
fi
sid=$(printf '%s' "$input" | jq -r '.session_id // empty')
dir="$HOME/.claude/session-notes"; f="$dir/$sid.txt"
case "$sid" in *[!A-Za-z0-9_-]*) sid="" ;; esac
# Our own block is untrusted text: drop control (Cc, incl. C1) and format (Cf, incl. bidi) characters; keeps tab and newline.
strip() { perl -CSD -pe 's/(?!\n|\t)[\p{Cc}\p{Cf}]//g' 2>/dev/null; }
if [ -n "$sid" ]; then
  if [ -f "$f" ]; then strip < "$f"; else echo "Objective: (set on your first prompt)"; fi
  qf="$dir/$sid.queue"; n=0; next=""
  [ -s "$qf" ] && { n=$(grep -c . "$qf"); next=$(head -1 "$qf" | strip | tr -d '\n' | perl -CSD -pe '$_=substr($_,0,90)' 2>/dev/null); }
  printf 'Queue (%s) [%s]%s\n' "$n" "${sid:0:4}" "${next:+  next: $next}"
fi
exit 0
