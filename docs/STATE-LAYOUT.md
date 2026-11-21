# State layout, version 0.5.0

Status: built. Reviewed; changes from the review: `migrate` links and removes instead of renaming (nothing is overwritten), merges a queue present in both places, moves notes last, refuses a link as `sessions`, tightens loose modes, and `/sitrep:setup` runs it after installing the program. Typed file kinds and `WriteFile(id, kind)` were not done.

## Purpose

The state folder `~/.claude/sitrep` holds eight or more files per session in one flat folder, `.txt`, `.cwd`, `.queue`, `.state`, `.count`, `.stamp`, `.pending`, `.pinned`, `.snooze` and `.lock`, so a folder of 13 sessions has 86 entries. This document moves each session into its own folder, and adds `sitrep migrate` to move existing state.

## The layout

```
~/.claude/sitrep/
  bin/sitrep
  sitrep.conf
  inbox.queue            the inbox is no session, so it stays here
  .queue.lock            the lock every queue change takes (one for the folder)
  sessions/
    <session id>/
      notes              was <id>.txt: Objective, Progress, Now
      queue              was <id>.queue
      cwd                was <id>.cwd
      state              was .<id>.state
      count              was .<id>.count
      stamp              was .<id>.stamp
      pending            was .<id>.pending
      lock               was .<id>.lock
      pinned             was .<id>.pinned
      snooze             was .<id>.snooze
      alias              a name for the session, one line (0.6.0; see ALIASES.md)
```

1. Modes are unchanged: folders `0700`, files `0600`. The `sessions` folder and each session folder are `0700`.
2. A session exists when its folder holds a regular file `notes`. A folder without one is not listed, resolved or pruned, as a flat `<id>.queue` without `<id>.txt` was not.
3. Every path comes from `state.Store`: `File(id, name)` gives `sessions/<id>/<name>`, and for the inbox `inbox.<name>` in the root. `Path` and `Hidden` stay as the names the callers know, and map the old extensions (`txt` becomes `notes`). A session folder is made by whatever writes first (`WriteFile`, an append, the pin file), with `0700`.
4. `WriteFile` accepts a path in the root or in a valid session folder only (invariant 3), and still writes a temporary file beside the target and renames.
5. The version is 0.5.0 in `plugin.json` and `internal/version`. Files of the old layout are ignored by 0.5.0 until they are migrated.

## `sitrep migrate`

`sitrep migrate [--dry-run]` moves the files of the old layout into the new one. It is run by the user after `/sitrep:setup`, never by a hook.

1. It looks only at names in the root that are `<id>.<ext>` or `.<id>.<ext>` for a valid session id (`[A-Za-z0-9_-]+`, not `inbox`) and one of the known extensions. Everything else is left alone: `bin`, `sitrep.conf`, `inbox.queue`, `.queue.lock`, `sessions`, unknown names, and anything that is not a regular file (never a link, a folder or a pipe).
2. Each file is renamed into `sessions/<id>/<name>` (`os.Rename`, so its modification time, which the state file's age is, is kept). When the destination exists the source stays where it is and is reported, so a re-run after an old hook wrote a flat file once more never overwrites newer state.
3. `.prompt` files, which the shell version wrote and nothing reads, are deleted. A `.lock` is moved like the rest.
4. It prints what it moved, left and deleted, and the counts. `--dry-run` prints the same and changes nothing. It exits 0 when nothing was left unmoved by an error; a file it could not move is named and the exit status is 1.
5. It is safe to run twice, and while old and new programs run side by side.

## What changes in the tests

1. The differential tests compare with the bash oracle, which writes the old layout. The harness reads both layouts as the old flat names, and runs `sitrep migrate` before each step of the Go side, so scenarios that seed old-layout files still work. That use is the harness's, not the program's.
2. Tests that name state files directly use the new names.
3. New tests: migration of every extension, of an inbox, of hostile names (a link, a folder, a pipe, a name with a dot, a path-like id), idempotence, a conflict, the dry run, modes and modification times kept, and a concurrent hook during a run.

## Rollback cost

A user who goes back to 0.4.x finds the new folder and no flat files; the old program starts a fresh state. There is no command to move back, and none is planned, since the only user has the copy that `migrate` leaves nowhere. Run `migrate --dry-run` first, and keep a copy of `~/.claude/sitrep` if the state matters.
