#!/bin/bash
# Wires claude-sitrep into ~/.claude/settings.json (with a timestamped backup) and links `q` into BIN_DIR (default ~/bin).
set -e
R=$(cd "$(dirname "$0")" && pwd)
S="$HOME/.claude/settings.json"; BIN_DIR=${BIN_DIR:-$HOME/bin}; CONF="$HOME/.claude/sitrep.conf"
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
command -v perl >/dev/null || { echo "perl is required" >&2; exit 1; }
umask 077
mkdir -p "$HOME/.claude/session-notes" "$BIN_DIR"
chmod 700 "$HOME/.claude/session-notes"; chmod 600 "$HOME"/.claude/session-notes/* 2>/dev/null || true
[ -f "$S" ] || echo '{}' > "$S"
cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
slp=$(printf '%q' "$R/statusline/statusline.sh"); hp=$(printf '%q' "$R/hooks/notes-hook.sh")
# The status line command carries a tag, so any clone's wrapper is recognised as ours. Installs from before the tag are
# recognised by their directory name.
tag="# claude-sitrep"
old=$(jq -r '.statusLine.command // empty' "$S")
tmp=$(mktemp "$S.XXXXXX"); trap 'rm -f "$tmp"' EXIT
jq --arg sl "bash $slp $tag" --arg h "bash $hp" --arg hp "$hp" '
  def ours: ((.command // "") | tostring | contains($hp));
  def without: map(if any(.hooks[]?; ours) then (.hooks |= map(select(ours | not))) | (if .hooks == [] then empty else . end) else . end);
  def add: ((. // []) | without) + [{hooks:[{type:"command",command:$h}]}];
  .statusLine = ((.statusLine // {}) + {type:"command",command:$sl})
  | .hooks.SessionStart |= add | .hooks.UserPromptSubmit |= add | .hooks.Stop |= add' "$S" > "$tmp"
cat "$tmp" > "$S"   # write through, so a symlinked settings.json stays a symlink
# Only after settings are written: keep the old status line (e.g. Claude HUD) as the upstream, unless it is ours.
shown=$(printf '%s' "$old" | perl -CSD -pe 's/[\p{Cc}\p{Cf}]//g' 2>/dev/null)
case "$old" in
  "") rm -f "$CONF"; echo "No existing status line found; showing the sitrep block only." ;;
  *"$slp"*|*"$tag"*|*claude-sitrep/statusline/statusline.sh*) echo "Status line is already sitrep; keeping the saved upstream (if any)." ;;
  *) printf 'SITREP_HUD_CMD=%q\n' "$old" > "$CONF"; echo "Wrapping the existing status line (it renders first): $shown" ;;
esac
if [ -e "$BIN_DIR/q" ] || [ -L "$BIN_DIR/q" ]; then
  [ "$(readlink "$BIN_DIR/q")" = "$R/bin/q" ] || echo "Note: $BIN_DIR/q exists and is not ours; left alone. Run $R/bin/q directly." >&2
else
  ln -s "$R/bin/q" "$BIN_DIR/q"
fi
echo "Installed. Restart Claude Code sessions to pick up the status line and hooks."
