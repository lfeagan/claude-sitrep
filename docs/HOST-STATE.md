# Host folders -- whether state should live under a host name

Status: decided, 2026-11-21, and the log is flattened (0.10.0). The rest of the state stays as it is. The note decides whether the history log keeps its `<host>` folder, and whether the rest of the state should move under one.

## Question

The history log writes to `log/<host>/current.jsonl`, a layout it took from Step 2 of `SHARED-STATE.md` (a folder per machine). That step was dropped, so we should ask whether the log still earns the folder, and whether sessions, queues and the inbox should follow it into `<host>/`.

## What Changed

`SHARED-STATE.md` measured a state folder on an SMB share: a hook that writes there takes 90 ms over Ethernet and 550 ms on Wi-Fi, against a budget of 30 ms. A hook can therefore never write to a shared folder, and the log is written by the hook (session start and end, `+`, `+pin`, `+take`). Thus a shared state folder is not a workable way to run several machines, and the answer for remote machines is ssh (`REMOTE.md`), where each machine keeps its own local state and its own locks.

Under that model a machine's files are only ever written by that machine. The folder name adds no information the entries do not carry already, as every entry has a `host` field.

## Options

1. Keep `log/<host>/`, and leave the rest of the state where it is. It costs nothing to run, but it keeps a layer whose reason is gone, and it makes `log/` the one place with a host in its path.
2. Flatten the log to `log/current.jsonl` and `log/previous.jsonl`, and keep `host` in each entry. A reader that wants several machines runs `sitrep log --json` on each over ssh and merges by time, as `REMOTE.md` does for `ls`. Every file has one writer, as the machine is the only writer.
3. Move everything under `<host>/` (sessions, queues, the inbox, `sitrep.conf`). Each part has a reason against it:
   1. Session ids are unique per machine already, so `sessions/<id>/` cannot collide.
   2. The inbox is the one file a person might want in common, and a host folder makes it per machine, which is the cross-machine view we dropped.
   3. The layout is a contract with the hooks, the status line, `sitrep migrate`, the completion and the differential tests against the bash version, and invariant 3 names its paths. A change reaches all of them, and its rollback is a second migration.

## Verdict

Option 2, and not option 3. Moving the rest buys nothing under the ssh model and costs a layout change in every reader.

The reason to decide now is that the log has not shipped. Version 0.10.0 is committed and not pushed, so no one has a `log/<host>/` folder, and flattening is a change to two path functions and their tests. After a release it would need a move of its own, and a reader that handled both.

## Rollback

Before a release, none: the change is in the commit. After one, a user who turned the log on has files in the old place, and a reader has to look in both until the 30-day window has passed.

## Open Questions

1. Whether two shells on one machine should still be told apart. They share the machine and a state folder, and `O_APPEND` writes of one line keep them apart, so we think not.
2. Whether `SITREP_STATE_DIR` on a shared folder should be allowed to write a log at all. It would be one more slow write in the hook. We could skip the log when the folder is not local, but we cannot tell that reliably, so for now the setting is the user's choice.
