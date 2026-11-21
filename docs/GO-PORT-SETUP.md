# Porting setup (Phase 7)

Status: draft 2, after review, and built. It follows `docs/GO-PORT.md`, which dropped the checkout installer and made setup plugin-only.

## Purpose

`sitrep setup` replaces `setup.sh`. After the plugin is installed and the binary is built, it puts the sitrep block on the status line, keeping any status line already there as the upstream, and links `sitrep` into `BIN_DIR`. `sitrep setup --remove` undoes exactly that. The plugin's own `hooks.json` brings the hooks, so setup writes none.

## What it touches

1. `~/.claude/settings.json`: only `statusLine`. The new command is `"$HOME/.claude/sitrep/bin/sitrep" statusline # claude-sitrep`. The trailing tag is how any clone's wrapper is recognised as ours (invariant 6). `$HOME` is expanded by the shell when Claude Code runs it, not by setup.
2. `~/.claude/sitrep/sitrep.conf`: only the line `SITREP_HUD_CMD=...`, which holds the upstream status line. Every other line is the user's and is kept. The file is removed when nothing else is left.
3. `~/.claude/sitrep/bin/sitrep`: the binary. When setup runs from another path (for example after `go install`), it copies itself there (temporary file, mode 0700, rename). The skill that builds from the plugin checkout writes straight to this path.
4. `BIN_DIR/sitrep` (default `~/bin`): a symlink to the binary, left alone with a note when something else is already there.

## Settings edits

1. Setup reads `settings.json`, checks `json.Valid` (the edit library accepts malformed input), and checks that `statusLine` is absent, null or an object. Otherwise it stops, changing nothing: `<path> cannot be parsed as JSON; nothing was changed.` or `The statusLine in <path> is not an object; nothing was changed.`
2. A backup `settings.json.bak.<timestamp>` is made first, mode 0600, from the resolved file.
3. `tidwall/sjson` sets `statusLine.type` and `statusLine.command` in place, so other keys, the order of keys, comments in the user's formatting and any other `statusLine` field (such as `padding`) stay as they are. A missing file becomes an indented document of its own, mode 0600.
4. The write goes to a temporary file in the directory of the resolved target (`EvalSymlinks`), with the target's mode, then a rename. A symlinked `settings.json` therefore stays a symlink and is never left half written. Nothing is changed before this step succeeds, and the state folder, conf and links are touched only after it.

## Cases

| Existing `statusLine.command` | What setup does |
| --- | --- |
| none | drop any saved upstream; say `No existing status line found; showing the sitrep block only.` |
| our command | keep the saved upstream; say `Status line is already set up; keeping the saved upstream (if any).` |
| anything else | save it as the upstream; say `Wrapping the existing status line (it renders first): <command, with control characters removed>` |

## The checkout guard (invariant 5)

Setup stops with exit 1, changing nothing, and names what it found, when any of these is present: a hook in `settings.json` whose command contains `hooks/sitrep-hook.sh` or `hooks/notes-hook.sh`; a status line command that ends in the tag but is not ours, or contains `claude-sitrep/statusline/statusline.sh`; a link `~/.claude/skills/sitrep-queue` pointing at a `*/skills/sitrep-queue`; a link in `BIN_DIR` pointing at a `*/bin/sitrep` that is not ours. The message says to remove those entries by hand, since the scripts that did it are gone. It also stops when `~/.claude/sitrep/plugin` exists and is not a link, or when `~/.claude/sitrep/bin/sitrep` is a directory.

## Remove

1. Backup, then: when the status line command is ours, restore the saved upstream (or delete `statusLine` when there is none), drop our conf line, and say `Status line restored.` Otherwise say `The status line is not sitrep's; left alone.`
2. Remove the `BIN_DIR` links that point at our binary and the binary itself, and the `bin` folder when it is empty. It never recurses over, and never `chmod`s, anything in the state folder (invariant 5: `plugin` and `bin` are not session files).
3. Say `Removed. State in ~/.claude/sitrep is left in place. Run /plugin uninstall sitrep to remove the plugin itself.`

## Testing

1. Fixtures of settings files: empty, `{}`, with other keys and a custom order, with `statusLine` null, a string, an object with `padding`, a HUD command, our own command, a symlinked file, malformed JSON, a very large integer value, CRLF line endings, a file without a trailing newline. Setup output is compared as text so key order and formatting are checked.
2. The frozen outputs of the bash `setup.sh` for the same cases (messages and exit codes) are the oracle for the words, not for the file contents, which change on purpose (the command, the missing link and copy).
3. Round trip: setup then remove restores the original settings byte for byte when there was an upstream, and restores everything but a trailing-newline difference when there was none.
4. Modes: backup, new settings, conf and binary are owner-only. A failure on any step before the settings write leaves every file as it was.
5. The guard: each kind of checkout leftover on its own, and several together.

## Rollback cost

Before Phase 8 nothing runs this. After it, `sitrep setup --remove` restores the status line, and the backups made by every run hold the prior settings.

## Decisions from the review

1. **The order.** Usage, then validity of settings, then the guard, then the shape checks (`bin` is a folder or absent, `bin/sitrep` is not a folder, `statusLine` is absent, null or an object with a string `command`), then the edit and its check, then the backup, then the settings write. The program is copied in after the settings are written, because the state folder may not exist before; if that copy fails the settings are put back and the run exits 1. Conf, the link and the messages come last.
2. **Settings checks.** A byte order mark is split off and put back. The root must be an object. A key that appears twice in the root or in `statusLine` is refused, because the edit library acts on the first of two equal keys and jq on the last. Comments and trailing commas are refused, as jq did. After the edit, `Verify` parses the result and requires every other top-level key to hold what it held and the status line to be what was meant; otherwise nothing is written.
3. **Backups.** They hold the bytes that were read, are created with `O_EXCL` at 0600, sit beside `settings.json` (not beside a link's target), and take a number when the name is taken, so a setup and a removal in the same second keep the pristine copy.
4. **The write.** A temporary file beside the real target, mode kept, synced, and just before the rename the target is read again; if it no longer holds what setup read, setup stops with `changed while it was being edited`. That narrows the race with Claude Code writing its own settings; it cannot close it without a lock both sides take. A dangling link is created where it points; a loop of links is an error.
5. **Conf.** Setup and its removal read `sitrep.conf` only, never the environment, so a `SITREP_HUD_CMD` in the shell is never written into settings. The reader understands what `printf %q` wrote (backslashes, single and double quotes and `$'...'`), checked against real bash in several locales. The writer uses single quotes, so bash can still source the file, and refuses a command with a line break, in which case setup says `Could not save the upstream status line to <conf>.` and names the backup that holds it, and carries on.
6. **Legacy.** A 0.3.x plugin setup (status line `bash "$HOME/.claude/sitrep/statusline.sh" # claude-sitrep`) counts as a checkout install here and stops setup. There is no migration; the author removes those entries, the `plugin` link and the copied `statusline.sh` by hand, as agreed. When a leftover checkout still has `uninstall.sh`, the message names the command to run; when it is gone it says to edit by hand.
7. **Remove.** It deletes only the program (one `os.Remove`, then the `bin` folder if empty), links that point at it, and puts back the status line. It never recurses over, or `chmod`s through, anything in the state folder. The hooks of the plugin keep naming the program, so removing before `/plugin uninstall sitrep` makes them fail without blocking until the plugin is gone.
8. **Round trip.** Setup then remove restores the settings semantically, and byte for byte when no character of the upstream command is escaped differently by JSON (for example `&&` is kept as it is, which the tests show).
