#!/bin/bash
# Removes our hooks, restores the upstream status line saved in ~/.claude/sitrep/sitrep.conf, and unlinks `sitrep` (and any alias) and the sitrep-queue skill if they are ours.
set -e
R=$(cd "$(dirname "$0")" && pwd)
S="$HOME/.claude/settings.json"; BIN_DIR=${BIN_DIR:-$HOME/bin}; SKILLS_DIR=${SKILLS_DIR:-$HOME/.claude/skills}; CONF="$HOME/.claude/sitrep/sitrep.conf"
# shellcheck source=lib/conf.sh
. "$R/lib/conf.sh"
cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
slp=$(printf '%q' "$R/statusline/statusline.sh"); hp=$(printf '%q' "$R/hooks/sitrep-hook.sh"); ohp=$(printf '%q' "$R/hooks/notes-hook.sh")
up=""
# shellcheck source=/dev/null
[ -f "$CONF" ] && { . "$CONF"; up=$SITREP_HUD_CMD; }
tmp=$(mktemp "$S.XXXXXX"); trap 'rm -f "$tmp"' EXIT
jq -L "$R/lib" --arg up "$up" --arg slp "$slp" --arg hp "$hp" --arg ohp "$ohp" '
  include "hooks";
  reduce ("SessionStart","UserPromptSubmit","Stop") as $e (.; .hooks[$e] |= ((. // []) | without($hp) | without($ohp)))
  | .hooks |= with_entries(select(.value | length > 0))
  | (if (.hooks | length) == 0 then del(.hooks) else . end)
  | if ((.statusLine.command // "") | contains($slp))
    then (if $up == "" then del(.statusLine) else .statusLine.command = $up end)
    else . end' "$S" > "$tmp"
cat "$tmp" > "$S"
hud_drop   # consumed above; a stale upstream would otherwise outlive us
for l in "$BIN_DIR"/*; do [ "$(readlink "$l" 2>/dev/null)" = "$R/bin/sitrep" ] && rm -f "$l"; done
[ "$(readlink "$SKILLS_DIR/sitrep-queue" 2>/dev/null)" = "$R/skills/sitrep-queue" ] && rm -f "$SKILLS_DIR/sitrep-queue"
echo "Uninstalled. State in ~/.claude/sitrep is left in place."
