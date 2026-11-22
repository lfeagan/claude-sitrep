# Listing and pruning sessions

The short version is in the [README](../README.md#listing-sessions).

## Listing sessions

`sitrep ls` prints a table of the running sessions: id, directory, state and queue, then Objective and Now, newest first by when the state last changed. `-v` adds the Progress line and lists each queue (and the inbox) under its session, at most 5 items each and then `+N more`. `-n <items>` changes the limit for one run and implies `-v`, and `-n 0` returns to the compact form. `SITREP_QUEUE_ITEMS` sets the default.

A session is running when a live process has its id in Claude Code's session registry (`~/.claude/sessions`). A session that is not registered but is under two minutes old stays listed, as the registry lags a start and a `/clear`. A session that has gone, or has said it ended, is hidden. The last lines say how many are hidden (`+2 ended (--all)`) and how many queue items they still hold (`3 items stranded in ended sessions (sitrep prune)`). When the registry cannot be read, a session counts as over once it has said `ended`.

The state comes from the hooks:

1. `working` from the moment a prompt goes to Claude.
2. `waiting` when Claude has stopped and it is your turn.
3. `blocked` while a working session waits at a permission dialog. The `PermissionRequest` hook records it, and it ends by itself once Claude writes to the transcript again, which it does when you answer (and the next prompt, `Stop` or exit remove the record). The state stays `working` in the JSON, with `needs_input` true and `blocked` set to `permission`. It is shown only for the default permission mode and the others that ask: in `auto` mode there is no dialog.
4. `ended` when the session exits.

The number after the state is how long ago it changed. Claude Code fires no hook when you interrupt a turn, so a `working` session whose transcript has been quiet for ten minutes shows as `idle?` with how long it has been quiet. A session with no state file shows `-`.

If the state folder cannot be read, such as a stale network mount, `ls`, `status` and `dash` say `cannot read the state folder` and exit 1, and do not show an empty list. A folder that does not exist yet is not an error. A mount that hangs is not caught. The hooks and the status line stay quiet on purpose, so that they never hold a prompt.

## Pruning

`sitrep prune [-y | -k | --dry-run]` tidies sessions that are gone. It works as follows:

1. It reads the live-session registry, and changes nothing if it cannot.
2. It marks each session that is not running as `ended`.
3. For a session that still holds queue items, it asks `Move N item(s) from <id> (<directory>) to the inbox? [y/N]`. `-y` answers yes for all. Without a terminal and without `-y` the items stay where they are, and it says so.
4. `--dry-run` prints what it would do and changes nothing, and `-k` or `--keep` marks the sessions ended and leaves every queue alone without asking, for a program that has no one to answer. The three options exclude one another.

It never touches a running session or one under two minutes old, and it never deletes a queue file.
