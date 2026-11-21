# History log -- an append-only record of changes

Status: built (0.10.0). The settings are `SITREP_LOG`, `SITREP_LOG_MAX` (megabytes) and `SITREP_LOG_DAYS` in `sitrep.conf` or the environment, in the style of the other settings. A move names its queues in `id` and `dest`, and a reorder names its positions in `from` and `to`. The host name is made safe for a folder name (`mbp.local` becomes `mbp_local`). Session starts that come from a compaction are not recorded, and an unpin or an end of a snooze is recorded only when there was something to end.

## Claim

We should keep a log of every change sitrep makes to its state: one line appended for each create, update and delete, and none for a read. The files stay the source of truth. The log is a record beside them, so we can answer "what did I queue last week" or "how long did this item wait in the inbox" without a database.

## Why a log and not SQLite

We looked at SQLite for history and queries and decided against it as the store: it costs us the file invariants, the plain-text state and the shared-folder design (see `SHARED-STATE.md`, where a database file on a synced drive is the worst case). An append-only text file keeps all three. It is also what the shared folder wants: a file with one writer.

## What goes in it

One line of JSON per change, written after the change succeeded:

```
{"e":"9f3a1c20","t":"2026-11-21T14:03:09Z","host":"mbp","id":"c5d0...","op":"queue.add","text":"fix the flaky test"}
```

The operations are the ones that change state: `queue.add`, `queue.rm`, `queue.edit`, `queue.move`, `queue.reorder`, `pin`, `unpin`, `snooze`, `alias`, `prune`. A session's start and end are worth recording as well. The summarizer's rewrites of notes are not, because they are frequent and say little.

`id` is a `state.ID`, or `inbox`. `text` has already been cleaned (invariant 1), and is cut at the same 200 characters as an item. `e` is a short random id for the entry.

## Entries describe what happened

An entry records the outcome, not the command, so that it still means something a week later and does not depend on the state it was written against. A command such as `top 3` or `rm 2` names a position, and a position is only meaningful at that moment.

- **Items by text, with positions as facts.** `queue.rm` carries the removed item's text. `queue.reorder` carries the text, `from` and `to`. `sitrep top 3` is recorded as a `queue.reorder` with `from` 3 and `to` 1, so it is still in the log, as what it did.
- **One entry for one change.** `queue.move` is a single entry with both queues, `id` and `dest`, and not a removal followed by an addition, so a reader never sees an item vanish and appear. `queue.edit` carries the old and the new text, which lets a reader follow an item through an edit.
- **A repeated entry is recognisable.** If an append fails partway and is retried, or a hook runs twice, the same line can appear twice. A reader drops a line whose `e` it has already seen. Each host has one writer per file, so sync cannot make a copy.
- **Nothing is replayed.** The log is a history. No code applies its entries to state, so none of them needs to be safe to apply twice.

The limit is that an item has no identity beyond its text. Two items with the same text look alike, and a reader following "how long did this wait in the inbox" matches on text and the edit chain, and can be wrong. Giving each item an id would fix it, but it changes the queue format, which is a bigger decision than this proposal should make.

## Where it lives

Two files per host in the state folder: `log/<host>/current.jsonl` and `log/<host>/previous.jsonl`. A host appends to its own `current` only, so every file has one writer. That is the same rule the shared-folder proposal uses, and a reader on another machine can merge the hosts' files by time. Both names are fixed, so nothing walks the folder and invariant 5 (no recursion over the state folder) holds.

The files are `0600` in a `0700` folder (invariant 2), and are opened for append without following a link.

## Opt-in

The log is off until the user turns it on with `SITREP_LOG=on` in `sitrep.conf` (or in the environment). While it is off nothing is written and no `log/` folder is made. `sitrep setup` does not turn it on, and `sitrep help log` says what the log keeps. Turning it off stops writing and leaves the files. `sitrep log --clear` removes the two files of this host, each by its own path.

## Size

The log has two limits, a size and an age, and the one reached first wins: 10 MB or 30 days by default (`SITREP_LOG_MAX` and `SITREP_LOG_DAYS`). Each file may hold half of each limit. When `current` reaches 5 MB, or its first entry is 15 days old, it is renamed over `previous` and a new `current` begins. The two files together then never hold more than 10 MB, and never an entry older than 30 days. Old entries fall off the end, and a reader sees a window of recent history, not all of it.

The age is checked only when something is appended, so a log that sits idle keeps what it has until the next change. A reader therefore ignores entries older than `SITREP_LOG_DAYS`, and `sitrep log --clear` is the way to remove them from disk at once.

Rolling over happens under a lock with a bounded wait, taken only when a limit is reached, and the limit is checked again once the lock is held. If the lock is busy the entry is appended anyway and the roll happens on a later call, so a hook never waits for it. A reader may find `current` replaced between two reads, and treats that as normal.

## Writing

- A hook never fails or waits because of the log (invariant 4). On any error the entry is dropped and the hook goes on and exits 0. The append is one `write` call on a file opened with `O_APPEND`, which needs no lock on a local disk and fits in the 30 ms budget.
- A line is kept to a bounded size. A crash can still leave a torn last line.
- The entry is written after the state file is changed. A crash between the two loses an entry. That is acceptable for a history, and it is the reason the log must not be used to rebuild state.

## Reading

Reading goes through `state.ReadFile` and takes each line as untrusted (invariant 1). A line that does not parse is skipped, which covers a torn last line. A new command, `sitrep log`, shows recent entries and filters by session, queue and time. The dashboard does not need to read it at first.

## Questions

- The defaults. 10 MB and 30 days are the user's choice, and we should measure how many entries a heavy week produces to see which limit is reached first.
- Whether a window of two files is enough history. If it is not, the limits can be raised, and a longer record would need a different design.
- Whether `sitrep prune` and the dashboard keys write the same entries. They run `sitrep` commands, so they should, with nothing extra.
- Whether the drop box in `SHARED-STATE.md` writes an entry on receipt, naming the sender's host, which would give us a trail of where an item came from.

## Not proposed

- Rebuilding state by replaying the log. That is event sourcing, a much larger change, and it would make the log the source of truth with all the recovery rules that implies.
- Logging reads, the summarizer's notes, or transcripts.
- Sending the log anywhere. It stays in the state folder.
