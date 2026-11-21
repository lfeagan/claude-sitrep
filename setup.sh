#!/bin/bash
# Run by /sitrep:setup after the sitrep plugin is installed. The plugin brings the hooks and the skills; this adds what a plugin cannot declare:
# the sitrep block on the status line, a copy of the status line script under ~/.claude/sitrep, and `sitrep` in BIN_DIR (default ~/bin).
# `--remove` undoes exactly that. It stops, and changes nothing, while an install from a checkout (./install.sh) is still in place,
# because that install's hooks would run beside the plugin's.
set -e
usage() { echo "usage: setup.sh [--remove]" >&2; exit 1; }
remove=0
case "${1:-}" in "") ;; --remove) remove=1 ;; *) usage ;; esac
[ $# -le 1 ] || usage
R=$(cd "$(dirname "$0")" && pwd -P)
S="$HOME/.claude/settings.json"; BIN_DIR=${BIN_DIR:-$HOME/bin}; SKILLS_DIR=${SKILLS_DIR:-$HOME/.claude/skills}
ND="$HOME/.claude/sitrep"; CONF="$ND/sitrep.conf"; PL="$ND/plugin"; SL="$ND/statusline.sh"
# shellcheck source=lib/conf.sh
. "$R/lib/conf.sh"
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
command -v perl >/dev/null || { echo "perl is required" >&2; exit 1; }
umask 077
tag="# claude-sitrep"
# The status line runs a copy that lives in our own folder, so it keeps working when the plugin's versioned folder is replaced or removed.
# shellcheck disable=SC2016  # $HOME is meant to be expanded when Claude Code runs the command, not now
newcmd='bash "$HOME/.claude/sitrep/statusline.sh" '"$tag"
show() { printf '%s' "$1" | perl -CSD -pe 's/[\p{Cc}\p{Cf}]//g' 2>/dev/null; }   # text from settings is untrusted

if [ -f "$S" ] && ! jq -e . "$S" >/dev/null 2>&1; then echo "$S cannot be parsed as JSON; nothing was changed." >&2; exit 1; fi
if [ "$remove" = 1 ]; then
  if [ -f "$S" ]; then
    cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
    if [ "$(jq -r '.statusLine.command // empty' "$S")" = "$newcmd" ]; then
      up=""
      # shellcheck source=/dev/null
      [ -f "$CONF" ] && up=$( . "$CONF" >/dev/null 2>&1; printf '%s' "$SITREP_HUD_CMD" )
      tmp=$(mktemp "$S.XXXXXX"); trap 'rm -f "$tmp"' EXIT
      jq --arg up "$up" 'if $up == "" then del(.statusLine) else .statusLine.command = $up end' "$S" > "$tmp"
      cat "$tmp" > "$S"
      hud_drop || echo "Could not update $CONF." >&2
      echo "Status line restored."
    else
      echo "The status line is not sitrep's; left alone."
    fi
  fi
  [ -L "$PL" ] && rm -f "$PL"
  [ -f "$SL" ] && [ ! -L "$SL" ] && rm -f "$SL"
  for l in "$BIN_DIR"/*; do [ "$(readlink "$l" 2>/dev/null)" = "$PL/bin/sitrep" ] && rm -f "$l"; done
  echo "Removed. State in ~/.claude/sitrep is left in place. Run /plugin uninstall sitrep to remove the plugin itself."
  exit 0
fi

# An install from a checkout: its hooks would fire twice beside the plugin's, and its skill would load twice.
found=(); roots=()
if [ -f "$S" ]; then
  while IFS= read -r c; do
    [ -n "$c" ] || continue
    found+=("a hook in settings.json: $(show "$c")")
    r=${c#bash }; r=${r%/hooks/*}; roots+=("$(printf '%s' "$r" | sed 's/\\\(.\)/\1/g')")
  done < <(jq -r '[.hooks[]?[]?.hooks[]? | (.command // "") | tostring | select(contains("hooks/sitrep-hook.sh") or contains("hooks/notes-hook.sh"))] | .[]' "$S" 2>/dev/null)
  sl=$(jq -r '.statusLine.command // empty' "$S" 2>/dev/null || true)
  if [ -n "$sl" ] && [ "$sl" != "$newcmd" ] && { [[ $sl == *"$tag" ]] || [[ $sl == *claude-sitrep/statusline/statusline.sh* ]]; }; then
    found+=("the status line: $(show "$sl")")
    r=${sl#bash }; r=${r%%/statusline/statusline.sh*}; roots+=("$(printf '%s' "$r" | sed 's/\\\(.\)/\1/g')")
  fi
fi
# A link counts as ours when its target still has our layout, or is gone and was named for us: another tool may also be called sitrep.
if [ -L "$SKILLS_DIR/sitrep-queue" ]; then
  t=$(readlink "$SKILLS_DIR/sitrep-queue")
  if [[ $t == */skills/sitrep-queue ]] && { [ -f "$t/SKILL.md" ] || [[ $t == *claude-sitrep* ]]; }; then found+=("the skill link $(show "$SKILLS_DIR/sitrep-queue") -> $(show "$t")"); roots+=("${t%/skills/sitrep-queue}"); fi
fi
for l in "$BIN_DIR"/*; do
  [ -L "$l" ] || continue
  t=$(readlink "$l")
  if [[ $t == */bin/sitrep ]] && [ "$t" != "$PL/bin/sitrep" ] && { [ -f "${t%/bin/sitrep}/hooks/sitrep-hook.sh" ] || [[ $t == *claude-sitrep* ]]; }; then found+=("the link $(show "$l") -> $(show "$t")"); roots+=("${t%/bin/sitrep}"); fi
done
if [ ${#found[@]} -gt 0 ]; then
  echo "Found an install from a checkout of claude-sitrep. Its hooks would run twice beside the plugin's:" >&2
  for f in "${found[@]}"; do echo "  - $f" >&2; done
  echo "Nothing was changed. Take it out first, then run this setup again:" >&2
  while IFS= read -r r; do
    [ -n "$r" ] || continue
    if [ -f "$r/uninstall.sh" ]; then echo "  bash $(printf '%q' "$(show "$r")")/uninstall.sh" >&2
    else echo "  $(show "$r") is gone: remove its entries from ~/.claude/settings.json by hand (a backup is made by every install)" >&2; fi
  done < <(printf '%s\n' "${roots[@]}" | sort -u)
  exit 1
fi
if [ -e "$PL" ] && [ ! -L "$PL" ]; then echo "$PL exists and is not a link; leaving it alone." >&2; exit 1; fi

[ -f "$S" ] || echo '{}' > "$S"
jq -e '(.statusLine // {}) | type == "object"' "$S" >/dev/null 2>&1 || { echo "The statusLine in $S is not an object; nothing was changed." >&2; exit 1; }
cp "$S" "$S.bak.$(date +%Y%m%dT%H%M%S)"
old=$(jq -r '.statusLine.command // empty' "$S")
tmp=$(mktemp "$S.XXXXXX"); trap 'rm -f "$tmp"' EXIT
jq --arg sl "$newcmd" '.statusLine = ((.statusLine // {}) + {type:"command",command:$sl})' "$S" > "$tmp"
cat "$tmp" > "$S"   # write through, so a symlinked settings.json stays a symlink
# From here the settings are written: nothing above this line has changed anything.
mkdir -p "$ND"; chmod 700 "$ND"
case "$old" in
  "") hud_drop || echo "Could not update $CONF." >&2; echo "No existing status line found; showing the sitrep block only." ;;
  "$newcmd") echo "Status line is already set up; keeping the saved upstream (if any)." ;;
  *) hud_put "$old" || echo "Could not save the upstream status line to $CONF." >&2
     echo "Wrapping the existing status line (it renders first): $(show "$old")" ;;
esac
t=$(mktemp "$ND/.statusline.XXXXXX"); cat "$R/statusline/statusline.sh" > "$t"; mv "$t" "$SL"
t="$ND/.plugin.$$"; rm -f "$t"; ln -s "$R" "$t"
perl -e 'rename $ARGV[0], $ARGV[1] or exit 1' "$t" "$PL" || { rm -f "$t"; echo "Could not link $PL." >&2; exit 1; }   # rename does not follow the old link
mkdir -p "$BIN_DIR"
if [ -e "$BIN_DIR/sitrep" ] || [ -L "$BIN_DIR/sitrep" ]; then
  [ "$(readlink "$BIN_DIR/sitrep")" = "$PL/bin/sitrep" ] || echo "Note: $BIN_DIR/sitrep exists and is not ours; left alone. Run $PL/bin/sitrep directly." >&2
else
  ln -s "$PL/bin/sitrep" "$BIN_DIR/sitrep"
fi
echo "Set up. Restart Claude Code sessions to pick up the status line."
