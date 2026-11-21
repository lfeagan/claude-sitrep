# Shared state -- a custom state folder and a view across machines

Status: proposal, 2026-11-21. Nothing here is built. It replaces the idea in the inbox of a remote server (WebDAV or a proprietary one).

## Claim

We should let the state folder live anywhere, and let several machines share one folder through a synced drive (Google Drive, OneDrive, Dropbox, a NAS). We should not build a server. The state is already plain files, so a shared folder gives a remote view for free, and it keeps the property we value most: sitrep is local, files only, and has no daemon.

## Today

`state.Open` names the state folder, always `~/.claude/sitrep`. It is not configurable. The layout (0.5.0) is a folder per session under `sessions/`, the inbox and `sitrep.conf` in the root, and one `.queue.lock` for all queue changes. Locks are `flock`, and every file is written to a temporary name and published by rename.

## Goal

From one machine, watch the sessions of other machines, and put an item on a remote session's queue or on a remote inbox. The first goal is a view. The second is a write.

## Step 1 -- a state folder path

Add `SITREP_STATE_DIR`, and a matching key in `sitrep.conf`, that replaces `~/.claude/sitrep`. This is small and useful by itself (a NAS, an encrypted volume, a test folder). All the invariants carry over to the new path unchanged: owner-only modes, `state.ID` for names, regular files only, and reads only through `state.ReadFile`.

Open questions:
- Whether the setting lives in the environment, in `sitrep.conf`, or both. `sitrep.conf` is inside the state folder, so it cannot name the folder. The environment, or a small pointer file in `~/.claude`, has to come first.
- Whether `sitrep setup` and `sitrep migrate` should accept the new path, or refuse it until the folder has been opened once by hand.

## Step 2 -- a folder per machine

Put every machine under its own subtree of the shared root, for example `hosts/<host>/sessions/<id>/`. Each file then has exactly one writer: the machine that owns it. A machine never writes another machine's files.

Reading is the easy half. `sitrep ls` and `sitrep watch` read every host's subtree and show a HOST column. The snapshot (`internal/snapshot`) is the seam, so the dashboard and `ls --json` get the new field together. `ls --json` is a public contract at version 1, so the host field has to be added without breaking it.

Open questions:
- The host name: the machine's name, or a name the user sets. It becomes part of a path, so it needs a validated type like `state.ID`.
- How the dashboard treats a host that has not updated for a long time (the drive may simply be behind).
- Whether `jump` makes sense for a remote session. It does not, so the key should say so and not fail quietly.

## Step 3 -- queueing on a remote machine

A single queue file with a lock cannot work across machines. Sync clients do not share `flock`, and two machines editing one file produce a conflicted copy or a lost write. Invariant 8 only holds on one machine.

We propose a drop box. A sender never edits a queue. It writes one new file, with a unique name, into the owner's `drop/` folder. The owner merges those files into the real queue the next time it runs a hook or a command, under its own local lock, then removes them. No file has two writers, so nothing can conflict.

This changes how the queue is written, so it needs its own design pass before any code.

## What a sync folder breaks

- **Locks.** `flock` is local only. Covered by one writer per file and the drop box.
- **Conflicts.** Clients resolve concurrent edits with copies like `queue (conflicted copy)`, or by last writer wins. Covered the same way.
- **Atomic rename.** To another machine a rename can look like a delete and a create, so a reader may see a file missing or half written for a moment. Readers must treat a missing or unreadable file as "not there yet" and never as an error.
- **Latency.** An update can take seconds or minutes to arrive. That is fine for a dashboard. It is the reason a remote view must never sit in a hook, whose budget is 30 ms. Hooks write only to the local folder.
- **Modes.** Many sync clients ignore file modes, so the owner-only invariant cannot be checked or kept on the shared drive. We say so in `sitrep setup`, and the user decides.
- **Trust.** Anyone who can write the shared folder can put text in your queue and in what the dashboard prints. Invariant 1 already covers printing: strip control and format characters from everything read, including another host's files. The drop box needs the same limits on size and count that a local queue has.

## Order of work

1. The state folder path.
2. Per-host subtrees and a read-only view across hosts.
3. The drop box, after a design of its own.

Each step stands alone. We can stop after step 1 or step 2 and still have something worth shipping.

## Not proposed

- A server of any kind, WebDAV included. It brings accounts, a network protocol and a process to keep running, and the shared folder already covers the need.
- Two storage backends (SQLite and files), inbox item 1. A database file on a synced drive is the worst case for sync conflicts. If we need a second backend later, it is a separate decision.
- Syncing the transcripts. They are large, they are private, and the dashboard does not need them from another machine.
