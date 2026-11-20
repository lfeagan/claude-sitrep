#!/bin/bash
# Wires claude-sitrep into ~/.claude/settings.json (with a timestamped backup) and links `q` into BIN_DIR (default ~/bin).
set -e
R=$(cd "$(dirname "$0")" && pwd)
S="$HOME/.claude/settings.json"; BIN_DIR=${BIN_DIR:-$HOME/bin}
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
mkdir -p "$HOME/.claude/session-notes" "$BIN_DIR"; [ -f "$S" ] || echo '{}' > "$S"
cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
# Keep the existing status line (e.g. Claude HUD) as the upstream, unless it is already ours.
old=$(jq -r '.statusLine.command // empty' "$S")
case "$old" in *claude-sitrep*|"") ;; *) printf 'SITREP_HUD_CMD=%q\n' "$old" > "$HOME/.claude/sitrep.conf" ;; esac
hook="bash $R/hooks/notes-hook.sh"
jq --arg sl "bash $R/statusline/statusline.sh" --arg h "$hook" '
  .statusLine={type:"command",command:$sl}
  | .hooks.SessionStart=((.hooks.SessionStart // [])|map(select(.hooks[0].command|contains("notes-hook")|not)))+[{hooks:[{type:"command",command:$h}]}]
  | .hooks.UserPromptSubmit=((.hooks.UserPromptSubmit // [])|map(select(.hooks[0].command|contains("notes-hook")|not)))+[{hooks:[{type:"command",command:$h}]}]
  | .hooks.Stop=((.hooks.Stop // [])|map(select(.hooks[0].command|contains("notes-hook")|not)))+[{hooks:[{type:"command",command:$h}]}]' "$S" > "$S.tmp" && mv "$S.tmp" "$S"
ln -sf "$R/bin/q" "$BIN_DIR/q"
echo "Installed. Restart Claude Code sessions to pick up the status line and hooks."
