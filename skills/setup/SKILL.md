---
name: setup
description: Set up or remove sitrep after installing the sitrep plugin: build the program, add the sitrep block to the status line and link the `sitrep` command. Use when the user has just installed or updated the plugin, says the sitrep block is missing from the status line, or wants to remove sitrep's status line and links.
argument-hint: "[remove]"
disable-model-invocation: true
allowed-tools: Bash(go version), Bash(go build*), Bash(mkdir -p "$HOME/.claude/sitrep"), Bash("$HOME/.claude/sitrep/sitrep.new" setup*), Bash("$HOME/.claude/sitrep/bin/sitrep" setup*)
---

sitrep is one Go program. The plugin brings its source, its hooks and the `sitrep-queue` skill. This setup builds the program into `~/.claude/sitrep/bin/sitrep`, which the hooks and the status line run, adds the sitrep block to the status line (keeping any status line that is already there, such as Claude HUD, above it), and links `sitrep` into `~/bin`. It writes `~/.claude/settings.json`, so Claude Code asks first.

If the argument is `remove`, run `"$HOME/.claude/sitrep/bin/sitrep" setup --remove` and report what it printed. Then tell the user to run `/plugin uninstall sitrep`, because until then the plugin's hooks name a program that is gone.

Otherwise:

1. Run `go version`. It needs Go 1.26 or newer. If `go` is missing or older, tell the user to install it (for example `brew install go`) and stop. Change nothing.
2. Run `mkdir -p "$HOME/.claude/sitrep"`.
3. Build from the plugin's own folder, so that the program always matches the plugin, into a new file beside the program in use, so that a hook running now never sees half a file: `go build -C "${CLAUDE_PLUGIN_ROOT}" -o "$HOME/.claude/sitrep/sitrep.new" -mod=readonly -buildvcs=false ./cmd/sitrep`. The first build downloads the Go modules, so it needs the network. If it fails, show the first lines of the error and stop.
4. Run `"$HOME/.claude/sitrep/sitrep.new" setup` and report what it printed, in a few lines. Setup copies the program into `~/.claude/sitrep/bin/sitrep`, which replaces the old one in one step, and removes `sitrep.new`.

If setup says it found an install from a checkout, it has changed nothing. Show the user what it names and how to take it out. Run nothing that removes those entries unless the user asks, and then run setup again.

After a setup, the user restarts Claude Code sessions to see the status line. Run this setup again after every plugin update, so that the program matches the new plugin; until then the hooks run the older program. State in `~/.claude/sitrep` is kept by a removal.
