# Shared by install.sh and uninstall.sh. $hp is the shell-quoted path of one of our hook scripts.
def ours($hp): ((.command // "") | tostring | contains($hp));
def without($hp): map(
  if any(.hooks[]?; ours($hp))
  then (.hooks |= map(select(ours($hp) | not))) | (if .hooks == [] then empty else . end)
  else . end);
