# Install, update and remove

The short version is in the [README](../README.md#install). This page has the details.

## Install

sitrep is one Go program. It needs Go 1.26 or newer to build, and the `claude` CLI.

```
/plugin install sitrep --marketplace lfeagan/claude-sitrep
/sitrep:setup
```

The first line adds this repository as a marketplace and installs the plugin, which brings the program's source, the hooks, the `sitrep-queue` skill (as `/sitrep:sitrep-queue`) and the `sitrep-history` skill, which tells Claude how to read the history log. A plugin cannot build a program or declare a status line, so `/sitrep:setup` does the rest, after Claude Code asks about the settings change:

1. It builds the program from the plugin's own folder into `~/.claude/sitrep/bin/sitrep`, which the hooks and the status line run, so it always matches the plugin.
2. It points the status line at `"$HOME/.claude/sitrep/bin/sitrep" statusline`. An existing status line, such as Claude HUD, is saved as `SITREP_HUD_CMD` in `~/.claude/sitrep/sitrep.conf` and runs first.
3. It links `sitrep` into `~/bin` (`BIN_DIR` changes the folder), unless something else is there.

`sitrep setup` backs up `~/.claude/settings.json` (to `settings.json.bak.<time>`, owner-only, never over an earlier backup) and checks the edit before it writes anything. It changes only the `statusLine` key and leaves the rest of the file as it was, a symlinked file included. If it finds an install from a checkout of the old shell version, it stops, changes nothing, and names what to take out by hand.

sitrep runs on macOS and Linux; it uses file locks and process groups, so there is no Windows build. On Windows, use WSL. A native install ends in hook errors on every event until you run `/plugin uninstall sitrep`, and `/sitrep:setup` stops on Windows before it changes anything.

## Updating

When a newer release exists, the status line says so under the queue line (`Update: sitrep 0.6.0 is out (you have 0.5.0)`). Updating is three steps, because the hooks and the status line run the built program, and a plugin update does not rebuild it:

1. Update the plugin with `claude plugin marketplace update sitrep && claude plugin update sitrep@sitrep`, or from `/plugin` inside Claude Code.
2. Make the session you will run setup from load the new plugin: restart it, or run `/reload-plugins` in it. A session that was open during step 1 still has the old plugin loaded, and `/sitrep:setup` there builds the old program again.
3. Run `/sitrep:setup`, so that the program matches the new plugin. It is safe to run again. `sitrep version` prints the version of the program, and `.claude-plugin/plugin.json` the version of the plugin; if the program still has the old version, the session had not loaded the new plugin, so reload it and run setup again. Sessions that were already running keep the old hooks until they are restarted, and so does a `sitrep dash` that is open.

Until the last step the hooks run the older program, and if you skipped it after the first install they name a program that does not exist yet, so each hook ends with a non-blocking error and a `+` prompt goes to Claude as written.

From a clone, with no plugin: `git pull --tags && make install`. `make install` builds the program and runs its `setup`, as `/sitrep:setup` does. `go install github.com/lfeagan/claude-sitrep/cmd/sitrep@latest` builds the same program from the newest version tag, and `sitrep setup` then puts it in place, wraps the status line and links the command; the hooks come only with the plugin.

Once a day a hook starts `sitrep update-check`, which runs `git ls-remote --tags` against this repository and records the newest `vX.Y.Z` tag in `~/.claude/sitrep/update`. It downloads and installs nothing, and the status line only reads that file. It needs `git` and the network, and fails quietly without them. `SITREP_CHECK_UPDATES=0` turns it off.

## Upgrading from 0.3.x, and removing sitrep

The shell versions left a status line, hooks or links that setup will not run beside. Setup stops, changes nothing, and names them. For the 0.3.x plugin setup that is the status line `bash "$HOME/.claude/sitrep/statusline.sh" # claude-sitrep`: put back the status line you had before (the command saved in `~/.claude/sitrep/sitrep.conf` as `SITREP_HUD_CMD`), or delete the `statusLine` key, and run `/sitrep:setup` again. For an install from a checkout (`./install.sh`), remove its four hooks and its status line from `~/.claude/settings.json` the same way, and delete the links `~/bin/sitrep` and `~/.claude/skills/sitrep-queue`. State is kept, and its files have the same names.

To take sitrep out, run `/sitrep:setup remove` (it restores the saved upstream status line and removes the program and its links), then `/plugin uninstall sitrep`. State in `~/.claude/sitrep` stays. If Claude HUD's setup runs after sitrep's, it replaces the status line and the sitrep block disappears: run `/sitrep:setup` again.
