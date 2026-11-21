---
name: setup
description: Set up or remove the sitrep status line and the `sitrep` command after installing the sitrep plugin. Use when the user has just installed the plugin, says the sitrep block is missing from the status line, or wants to remove sitrep's status line and links.
argument-hint: "[remove]"
disable-model-invocation: true
allowed-tools: Bash(bash "${CLAUDE_PLUGIN_ROOT}/setup.sh"*)
---

The plugin's hooks and the `sitrep-queue` skill load by themselves. This setup does the rest: it adds the sitrep block to the status line, keeping any status line that is already there (such as Claude HUD) above it, and links `sitrep` into `~/bin` so it works in the user's own terminal. It writes `~/.claude/settings.json`, so Claude Code asks first.

If the argument is `remove`, run `bash "${CLAUDE_PLUGIN_ROOT}/setup.sh" --remove`. Otherwise run `bash "${CLAUDE_PLUGIN_ROOT}/setup.sh"`. Report what it printed, in a few lines.

If it says an install from a checkout was found, it has changed nothing. Show the user the `uninstall.sh` command it names, and run that only if the user asks. Then run this setup again.

After a setup, the user restarts Claude Code sessions to see the status line. To take sitrep out, run this with `remove` before `/plugin uninstall sitrep`. State in `~/.claude/sitrep` is kept.
