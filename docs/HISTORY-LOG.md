# History log -- an append-only record of changes

Status: proposal, 2026-11-21. Nothing here is built.

## Claim

We should keep a log of every change sitrep makes to its state: one line appended for each create, update and delete, and none for a read. The files stay the source of truth. The log is a record beside them, so we can answer "what did I queue last week" or "how long did this item wait in the inbox" without a database.

## Why a log and not SQLite

We looked at SQLite for history and queries and decided against it as the store: it costs us the file invariants, the plain-text state and the shared-folder design (see `SHARED-STATE.md`, where a database file on a synced drive is the worst case). An append-only text file keeps all three. It is also what the shared folder wants: a file with one writer.

## What goes in it

One line of JSON per change, written after the change succeeded:

```
{"t":"2026-11-21T14:03:09Z","host":"mbp","id":"c5d0...","op":"queue.add","text":"fix the flaky test"}
```

The operations are the ones that change state: `queue.add`, `queue.rm`, `queue.edit`, `queue.move` (with both queues), `queue.top`, `queue.reorder`, `pin`, `unpin`, `snooze`, `alias`, `prune`. A session's start and end are worth recording as well. The summarizer's rewrites of notes are not, because they are frequent and say little.

`id` is a `state.ID`, or `inbox`. `text` has already been cleaned (invariant 1), and is cut at the same 200 characters as an item.

## Where it lives

Two files per host in the state folder: `log/<host>/current.jsonl` and `log/<host>/previous.jsonl`. A host appends to its own `current` only, so every file has one writer. That is the same rule the shared-folder proposal uses, and a reader on another machine can merge the hosts' files by time. Both names are fixed, so nothing walks the folder and invariant 5 (no recursion over the state folder) holds.

The files are `0600` in a `0700` folder (invariant 2), and are opened for append without following a link.

## Opt-in

The log is off until the user turns it on with `log = on` in `sitrep.conf` (or `SITREP_LOG=1`). While it is off nothing is written and no `log/` folder is made. `sitrep setup` does not turn it on, and says what the log keeps when it is asked about it. Turning it off stops writing and leaves the files. `sitrep log --clear` removes the two files of this host, each by its own path.

## Size

The log has a hard limit, so it can never grow without bound. When `current` reaches a cap (1 MiB by default, `log_max` in `sitrep.conf`), it is renamed over `previous` and a new `current` begins. Two files of at most the cap are all a host ever holds, so with entries of a few hundred bytes the log keeps the last few thousand changes and never more than twice the cap. Old entries fall off the end, and a reader sees a window of recent history, not all of it.

Rolling over happens under a lock with a bounded wait, taken only when the size is over the cap, and the size is checked again once the lock is held. If the lock is busy the entry is appended anyway and the roll happens on a later call, so a hook never waits for it. A reader may find `current` replaced between two reads, and treats that as normal.

## Writing

- A hook never fails or waits because of the log (invariant 4). On any error the entry is dropped and the hook goes on and exits 0. The append is one `write` call on a file opened with `O_APPEND`, which needs no lock on a local disk and fits in the 30 ms budget.
- A line is kept to a bounded size. A crash can still leave a torn last line.
- The entry is written after the state file is changed. A crash between the two loses an entry. That is acceptable for a history, and it is the reason the log must not be used to rebuild state.

## Reading

Reading goes through `state.ReadFile` and takes each line as untrusted (invariant 1). A line that does not parse is skipped, which covers a torn last line. A new command, `sitrep log`, shows recent entries and filters by session, queue and time. The dashboard does not need to read it at first.

## Questions

- The default cap. 1 MiB is a guess, and we should measure how many entries a heavy week produces before settling on it.
- Whether a window of two files is enough history. If it is not, the cap can be raised, and a longer record would need a different design.
- Whether `sitrep prune` and the dashboard keys write the same entries. They run `sitrep` commands, so they should, with nothing extra.
- Whether the drop box in `SHARED-STATE.md` writes an entry on receipt, naming the sender's host, which would give us a trail of where an item came from.

## Not proposed

- Rebuilding state by replaying the log. That is event sourcing, a much larger change, and it would make the log the source of truth with all the recovery rules that implies.
- Logging reads, the summarizer's notes, or transcripts.
- Sending the log anywhere. It stays in the state folder.
