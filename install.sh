#!/bin/bash
# Wires claude-sitrep into ~/.claude/settings.json (with a timestamped backup) links `sitrep` into BIN_DIR (default ~/bin) and the sitrep-queue skill into SKILLS_DIR (default ~/.claude/skills).
# `--alias NAME` also links NAME (e.g. q) to `sitrep`, unless NAME is already taken.
set -e
ALIAS=""
case "$1" in
  --alias) ALIAS=$2; [[ $ALIAS =~ ^[A-Za-z0-9_-]+$ ]] || { echo "usage: install.sh [--alias NAME]" >&2; exit 1; } ;;
  "") ;;
  *) echo "usage: install.sh [--alias NAME]" >&2; exit 1 ;;
esac
R=$(cd "$(dirname "$0")" && pwd)
S="$HOME/.claude/settings.json"; BIN_DIR=${BIN_DIR:-$HOME/bin}; SKILLS_DIR=${SKILLS_DIR:-$HOME/.claude/skills}; ND="$HOME/.claude/sitrep"; CONF="$ND/sitrep.conf"
# shellcheck source=lib/conf.sh
. "$R/lib/conf.sh"
if [ -L "$ND/plugin" ] || [ -e "$ND/plugin" ]; then
  echo "The sitrep plugin is set up here ($ND/plugin). Its hooks would run twice beside these. Run /sitrep:setup with remove first, or leave the plugin as it is." >&2
  exit 1
fi
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
command -v perl >/dev/null || { echo "perl is required" >&2; exit 1; }
umask 077
[ -f "$S" ] || echo '{}' > "$S"
cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
slp=$(printf '%q' "$R/statusline/statusline.sh"); hp=$(printf '%q' "$R/hooks/sitrep-hook.sh"); ohp=$(printf '%q' "$R/hooks/notes-hook.sh")   # ohp: the name before 0.2, dropped so an upgrade leaves one hook
# The tag on our status line command is how any clone's wrapper is recognised as ours; also match untagged commands from earlier installs.
tag="# claude-sitrep"
old=$(jq -r '.statusLine.command // empty' "$S")
had=$(jq -r --arg hp "$hp" '[.hooks.SessionEnd[]?.hooks[]? | (.command // "") | tostring | select(contains($hp))] | length' "$S" 2>/dev/null)
tmp=$(mktemp "$S.XXXXXX"); trap 'rm -f "$tmp"' EXIT
jq -L "$R/lib" --arg sl "bash $slp $tag" --arg h "bash $hp" --arg hp "$hp" --arg ohp "$ohp" '
  include "hooks";
  def add: ((. // []) | without($hp) | without($ohp)) + [{hooks:[{type:"command",command:$h}]}];
  .statusLine = ((.statusLine // {}) + {type:"command",command:$sl})
  | reduce ("SessionStart","UserPromptSubmit","Stop","SessionEnd") as $e (.; .hooks[$e] |= add)' "$S" > "$tmp"
cat "$tmp" > "$S"   # write through, so a symlinked settings.json stays a symlink
if [ "${had:-0}" = 0 ]; then echo "Registered the SessionEnd hook, which marks finished sessions."; fi
# Earlier versions kept state in ~/.claude/session-notes and the config in ~/.claude/sitrep.conf; move them only if every file there is ours.
od="$HOME/.claude/session-notes"
if [ -d "$od" ] && [ -z "$(find "$od" -mindepth 1 ! \( -name '*.txt' -o -name '*.queue' -o -name '*.cwd' -o -name '.*.prompt' -o -name '.*.count' -o -name '.*.state' -o -name '.*.stamp' -o -name '.*.pending' \) 2>/dev/null)" ]; then
  mkdir -p "$ND"   # a running hook may have created it already, so merge and never overwrite
  for x in "$od"/* "$od"/.[!.]*; do [ -f "$x" ] && mv -n "$x" "$ND/"; done
  rmdir "$od" 2>/dev/null && echo "Moved state from $od to $ND."
fi
mkdir -p "$ND"; chmod 700 "$ND"; find "$ND" -maxdepth 1 -type f -exec chmod 600 {} + 2>/dev/null || true   # files only: a link in here must not be followed
[ -e "$HOME/.claude/sitrep.conf" ] && [ ! -e "$CONF" ] && mv "$HOME/.claude/sitrep.conf" "$CONF"
# Only after settings are written: keep the old status line (e.g. Claude HUD) as the upstream, unless it is ours.
case "$old" in
  "") hud_drop || echo "Could not update $CONF." >&2; echo "No existing status line found; showing the sitrep block only." ;;
  *"$slp"*|*"$tag"*|*claude-sitrep/statusline/statusline.sh*) echo "Status line is already sitrep; keeping the saved upstream (if any)." ;;
  *) hud_put "$old" || echo "Could not save the upstream status line to $CONF." >&2
     echo "Wrapping the existing status line (it renders first): $(printf '%s' "$old" | perl -CSD -pe 's/[\p{Cc}\p{Cf}]//g' 2>/dev/null)" ;;
esac
mkdir -p "$BIN_DIR" "$SKILLS_DIR"
link() { # name: link BIN_DIR/name to bin/sitrep unless something else owns it
  if [ -e "$BIN_DIR/$1" ] || [ -L "$BIN_DIR/$1" ]; then
    [ "$(readlink "$BIN_DIR/$1")" = "$R/bin/sitrep" ] || echo "Note: $BIN_DIR/$1 exists and is not ours; left alone. Run $R/bin/sitrep directly." >&2
  else
    ln -s "$R/bin/sitrep" "$BIN_DIR/$1"
  fi
}
link sitrep
# The command was called `q` before 0.2; drop its link from this checkout, which now dangles.
[ -L "$BIN_DIR/q" ] && [ ! -e "$BIN_DIR/q" ] && [ "$(readlink "$BIN_DIR/q")" = "$R/bin/q" ] && rm -f "$BIN_DIR/q" && echo "Removed the old q link."
[ -z "$ALIAS" ] || link "$ALIAS"
sk="$SKILLS_DIR/sitrep-queue"
if [ -e "$sk" ] || [ -L "$sk" ]; then
  [ "$(readlink "$sk")" = "$R/skills/sitrep-queue" ] || echo "Note: $sk exists and is not ours; left alone." >&2
else
  ln -s "$R/skills/sitrep-queue" "$sk"
fi
echo "Installed. Restart Claude Code sessions to pick up the status line and hooks."
