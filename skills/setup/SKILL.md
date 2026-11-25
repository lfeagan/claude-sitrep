---
name: setup
description: Set up or remove sitrep after installing the sitrep plugin: build the program, add the sitrep block to the status line and link the `sitrep` command. Use when the user has just installed or updated the plugin, asks how to update sitrep, says the sitrep block is missing from the status line, or wants to remove sitrep's status line and links.
argument-hint: "[remove]"
disable-model-invocation: true
allowed-tools: Bash(uname -s), Bash(go version), Bash(GOWORK=off go build -C *), Bash(mkdir -p "$HOME/.claude/sitrep"), Bash("$HOME/.claude/sitrep/sitrep.new" setup*), Bash("$HOME/.claude/sitrep/bin/sitrep" setup*)
---

sitrep is one Go program. The plugin brings its source, its hooks and the `sitrep-queue` skill. This setup builds the program into `~/.claude/sitrep/bin/sitrep`, which the hooks and the status line run, adds the sitrep block to the status line (keeping any status line that is already there, such as Claude HUD, above it), and links `sitrep` into `~/bin`. It writes `~/.claude/settings.json`, so Claude Code asks first.

First run `uname -s`. `Darwin` and `Linux` (WSL prints `Linux`) go on. For anything else, such as `MINGW`, `MSYS` or `CYGWIN`, or when `uname` is not found, stop and change nothing: tell the user that sitrep runs on macOS and Linux only, since it uses file locks and process groups that Windows does not have, and that WSL is the way to use it there. The hooks of the installed plugin will report an error on every event until the plugin is removed, so tell them to run `/plugin uninstall sitrep`.

If the argument is `remove`, run `"$HOME/.claude/sitrep/bin/sitrep" setup --remove` and report what it printed. Then tell the user to run `/plugin uninstall sitrep`, because until then the plugin's hooks name a program that is gone.

Otherwise:

1. Run `go version`. It needs Go 1.26 or newer. If `go` is missing or older, tell the user to install it (for example `brew install go`) and stop. Change nothing.
2. Run `mkdir -p "$HOME/.claude/sitrep"`.
3. Build from the plugin's own folder, so that the program always matches the plugin, into a new file beside the program in use, so that a hook running now never sees half a file: `GOWORK=off go build -C "${CLAUDE_PLUGIN_ROOT}" -o "$HOME/.claude/sitrep/sitrep.new" -mod=readonly -buildvcs=false ./cmd/sitrep`. `GOWORK=off` keeps a `go.work` file in a folder above the plugin from changing what is built. The first build downloads the Go modules, so it needs the network. If it fails, show the first lines of the error and stop.
4. Run `"$HOME/.claude/sitrep/sitrep.new" setup` and report what it printed, in a few lines. If it says a newer sitrep is out, pass that on, with what to do: this session still has the old plugin loaded, so restart it or run `/reload-plugins`, then run this setup again. Setup copies the program into `~/.claude/sitrep/bin/sitrep`, which replaces the old one in one step, and removes `sitrep.new`.

If setup says it found an install from a checkout, it has changed nothing. Show the user what it names and how to take it out. Run nothing that removes those entries unless the user asks, and then run setup again.

After a setup, the user restarts Claude Code sessions to see the status line. State in `~/.claude/sitrep` is kept by a removal.

To update sitrep, the user updates the plugin first (`claude plugin marketplace update sitrep && claude plugin update sitrep@sitrep`, or `/plugin`) and restarts Claude Code, then runs this setup again, so that the program matches the new plugin; until then the hooks run the older program. The status line says "Update: sitrep X is out" when a newer release exists. A user with a clone updates with `git pull --tags && make install`.
