# shellcheck shell=bash
# Shared by install.sh and uninstall.sh. sitrep.conf holds our SITREP_HUD_CMD line and may hold settings the user added
# (such as SITREP_QUEUE_ITEMS), so only that one line is ever replaced or removed. Needs $CONF.
hud_drop() { # remove our line, and the file when nothing else is left
  [ -f "$CONF" ] || return 0
  local t; t=$(mktemp "$CONF.XXXXXX") || return 1
  { grep -v '^SITREP_HUD_CMD=' "$CONF" || true; } > "$t"
  if grep -q . "$t"; then cat "$t" > "$CONF"; else rm -f "$CONF"; fi
  rm -f "$t"
}
hud_put() { # command: save it as the upstream status line
  hud_drop; printf 'SITREP_HUD_CMD=%q\n' "$1" >> "$CONF"
}
