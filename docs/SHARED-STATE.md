# Shared state -- a custom state folder and a view across machines

Status: proposal, 2026-11-21. Step 1 is built (0.9.0); the rest is not. It replaces the idea in the inbox of a remote server (WebDAV or a proprietary one).

## Claim

We should let the state folder live anywhere, and let several machines share one folder through a synced drive (Google Drive, OneDrive, Dropbox, a NAS). We should not build a server. The state is already plain files, so a shared folder gives a remote view for free, and it keeps the property we value most: sitrep is local, files only, and has no daemon.

## Today

`state.Open` names the default state folder, `~/.claude/sitrep`, and since 0.9.0 `SITREP_STATE_DIR` can move it (Step 1). The layout (0.5.0) is a folder per session under `sessions/`, the inbox and `sitrep.conf` in the root, and one `.queue.lock` for all queue changes. Locks are `flock`, and every file is written to a temporary name and published by rename.

## Goal

From one machine, watch the sessions of other machines, and put an item on a remote session's queue or on a remote inbox. The first goal is a view. The second is a write.

## Step 1 -- a state folder path (built in 0.9.0)

`SITREP_STATE_DIR` replaces `~/.claude/sitrep`. It is small and useful by itself (a NAS, an encrypted volume, a demo, a test folder). All the invariants carry over to the new path unchanged: owner-only modes, `state.ID` for names, regular files only, and reads only through `state.ReadFile`.

What we decided:
- **The environment only.** `sitrep.conf` is inside the state folder, so it cannot name the folder, and a pointer file in `~/.claude` would be a second place to look. The hooks, the status line and the summarizer take the variable from the shell that starts `claude`.
- **Only the state moves.** The program stays in `~/.claude/sitrep/bin`, because the plugin's hooks run it from there.
- **A bad value does nothing, and never falls back.** A value that is not an absolute path (a relative path, `~/x`), or that is the home folder, Claude's own folder or a folder above them, is refused: a hook exits 0 and does nothing, the status line says what is wrong, and every other command fails. Falling back to the default folder would write the state of a demo into the real one.
- **`setup` and `migrate` refuse** while it is set, and change nothing. They work on `~/.claude/sitrep`.

### What we measured

On 2026-11-21 we ran the 0.9.0 program with `SITREP_STATE_DIR` on a Synology share mounted over SMB, from one Mac, first on Wi-Fi and then over gigabit Ethernet (a ping of under 1 ms). The numbers are medians of 15 runs, in milliseconds. Each run was checked to have reached the share.

| | SMB over Wi-Fi | SMB over Ethernet | local disk |
|---|---|---|---|
| hook (several writes) | 550 | 88 | 3 |
| status line | 13 | 4 | 3 |
| `ls` | 41 | 8 | 3 |

What worked: `add`, `ls` and the hook; `flock` between processes on the one Mac (40 parallel `add` commands lost nothing, and a held lock gave the bounded wait and then `busy`); and the write-then-rename. The mount reports every folder as `0700` and every file as `0700`, so modes are emulated, and `Ensure` had nothing to tighten. What did not work is the hook budget (30 ms): every prompt would wait about half a second on Wi-Fi, and about 90 ms on Ethernet, three times the budget. Reads are fast enough on Ethernet. Locking between two machines is untested. A mount that has gone away is also a risk: `sitrep ls` on a stale mount printed an empty queue and exited 0, so an unreadable state folder looks like an empty one.

So a state folder on a network mount is usable to read, and wrong for a live session to write, even on a good wired link. The next two steps are designed so that no hook ever writes to a shared folder.

## Step 2 -- a folder per machine

Each machine keeps its own state in a local folder, as it does now, so a hook never waits on a network. A detached `publish` child, started by a hook at most once in a while and kept to a deadline and its own process group like `update-check` (invariant 9), copies what other machines need into its own subtree of the shared root, for example `hosts/<host>/sessions/<id>/`. Each file then has exactly one writer: the machine that owns it. A machine never writes another machine's files.

Reading is the easy half. `sitrep ls` and `sitrep watch` read every host's subtree and show a HOST column. The snapshot (`internal/snapshot`) is the seam, so the dashboard and `ls --json` get the new field together. `ls --json` is a public contract at version 1, so the host field has to be added without breaking it.

Open questions:
- The host name: the machine's name, or a name the user sets. It becomes part of a path, so it needs a validated type like `state.ID`.
- How the dashboard treats a host that has not updated for a long time (the drive may simply be behind).
- Whether `jump` makes sense for a remote session. It does not, so the key should say so and not fail quietly.

## Step 3 -- queueing on a remote machine

A single queue file with a lock cannot work across machines. Sync clients do not share `flock`, and two machines editing one file produce a conflicted copy or a lost write. Invariant 8 only holds on one machine.

We propose a drop box. A sender never edits a queue. It writes one new file, with a unique name, into the owner's `drop/` folder. The owner merges those files into the real queue, under its own local lock, then removes them. The owner does that in the same detached step that publishes, and not in the hook itself, since reading the shared folder costs a round trip. No file has two writers, so nothing can conflict.

This changes how the queue is written, so it needs its own design pass before any code.

## What a sync folder breaks

- **Locks.** `flock` is local only. Covered by one writer per file and the drop box.
- **Conflicts.** Clients resolve concurrent edits with copies like `queue (conflicted copy)`, or by last writer wins. Covered the same way.
- **Atomic rename.** To another machine a rename can look like a delete and a create, so a reader may see a file missing or half written for a moment. Readers must treat a missing or unreadable file as "not there yet" and never as an error.
- **Latency.** An update can take seconds or minutes to arrive. That is fine for a dashboard. It is the reason a remote view must never sit in a hook, whose budget is 30 ms. Hooks write only to the local folder. The measurement above shows why: a hook over SMB took 550 ms.
- **Modes.** Many sync clients ignore file modes, so the owner-only invariant cannot be checked or kept on the shared drive. We say so in `sitrep setup`, and the user decides.
- **Trust.** Anyone who can write the shared folder can put text in your queue and in what the dashboard prints. Invariant 1 already covers printing: strip control and format characters from everything read, including another host's files. The drop box needs the same limits on size and count that a local queue has.

## Order of work

1. The state folder path (done, 0.9.0).
2. A local state folder published to per-host subtrees, and a read-only view across hosts.
3. The drop box, after a design of its own.

Each step stands alone. We can stop after step 1 or step 2 and still have something worth shipping.

## Not proposed

- A server of any kind, WebDAV included. It brings accounts, a network protocol and a process to keep running, and the shared folder already covers the need.
- Two storage backends (SQLite and files), inbox item 1. A database file on a synced drive is the worst case for sync conflicts. If we need a second backend later, it is a separate decision.
- Syncing the transcripts. They are large, they are private, and the dashboard does not need them from another machine.
