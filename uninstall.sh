#!/bin/bash
# Removes the hooks, restores the upstream status line saved in ~/.claude/sitrep.conf, and unlinks `q`.
set -e
S="$HOME/.claude/settings.json"; BIN_DIR=${BIN_DIR:-$HOME/bin}
cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
up=""; [ -f "$HOME/.claude/sitrep.conf" ] && { . "$HOME/.claude/sitrep.conf"; up=$SITREP_HUD_CMD; }
jq --arg up "$up" '
  .hooks.SessionStart|=((. // [])|map(select(.hooks[0].command|contains("notes-hook")|not)))
  | .hooks.UserPromptSubmit|=((. // [])|map(select(.hooks[0].command|contains("notes-hook")|not)))
  | .hooks.Stop|=((. // [])|map(select(.hooks[0].command|contains("notes-hook")|not)))
  | if $up=="" then del(.statusLine) else .statusLine={type:"command",command:$up} end' "$S" > "$S.tmp" && mv "$S.tmp" "$S"
rm -f "$BIN_DIR/q"; echo "Uninstalled. Notes in ~/.claude/session-notes are left in place."
