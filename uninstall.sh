#!/bin/bash
# Removes our hooks, restores the upstream status line saved in ~/.claude/sitrep.conf, and unlinks `q` if it is ours.
set -e
R=$(cd "$(dirname "$0")" && pwd)
S="$HOME/.claude/settings.json"; BIN_DIR=${BIN_DIR:-$HOME/bin}
cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
slp=$(printf '%q' "$R/statusline/statusline.sh"); hp=$(printf '%q' "$R/hooks/notes-hook.sh")
up=""
# shellcheck source=/dev/null
[ -f "$HOME/.claude/sitrep.conf" ] && { . "$HOME/.claude/sitrep.conf"; up=$SITREP_HUD_CMD; }
tmp=$(mktemp "$S.XXXXXX"); trap 'rm -f "$tmp"' EXIT
jq --arg up "$up" --arg slp "$slp" --arg hp "$hp" '
  def ours: ((.command // "") | tostring | contains($hp));
  def without: map(if any(.hooks[]?; ours) then (.hooks |= map(select(ours | not))) | (if .hooks == [] then empty else . end) else . end);
  .hooks.SessionStart |= ((. // []) | without)
  | .hooks.UserPromptSubmit |= ((. // []) | without)
  | .hooks.Stop |= ((. // []) | without)
  | .hooks |= with_entries(select(.value | length > 0))
  | (if (.hooks | length) == 0 then del(.hooks) else . end)
  | if ((.statusLine.command // "") | contains($slp))
    then (if $up == "" then del(.statusLine) else .statusLine.command = $up end)
    else . end' "$S" > "$tmp"
cat "$tmp" > "$S"
rm -f "$HOME/.claude/sitrep.conf"   # consumed above; a stale upstream would otherwise outlive us
[ "$(readlink "$BIN_DIR/q" 2>/dev/null)" = "$R/bin/q" ] && rm -f "$BIN_DIR/q"
echo "Uninstalled. Notes in ~/.claude/session-notes are left in place."
