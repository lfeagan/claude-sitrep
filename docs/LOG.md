# The history log

The format of `sitrep log --json`, which programs read. `docs/HISTORY-LOG.md` is the design note behind it.

With `SITREP_LOG=on`, sitrep appends one line to `~/.claude/sitrep/log/current.jsonl` for each change it makes: a queue add, remove, edit, move or reorder, a pin or unpin, a snooze, an alias, a prune, and the start and end of a session. Reads and the summarizer's rewrites of the notes are not recorded. An entry says what happened, with the text of the item, so it still means something a week later. `sitrep top 3` is recorded as a reorder from 3 to 1.

`sitrep log` shows the latest entries, and `--session`, `--op`, `--host`, `--since` and `-n` narrow them (`--json` prints one object per line). The log is a record beside the files and nothing replays it: the files stay the source of truth. It keeps two files (`current.jsonl` and `previous.jsonl`), so it holds at most `SITREP_LOG_MAX` megabytes and no entry older than `SITREP_LOG_DAYS` days. Each entry names its host, so logs from several machines (over ssh, as [REMOTE.md](REMOTE.md) does for `ls`) can be told apart. `sitrep log --clear` deletes the files. Nothing sends the log anywhere. Because it keeps the text of your items, it is off until you turn it on. The `sitrep-history` skill tells Claude how to search it, so you can ask what you queued last week.

Each line of `sitrep log --json` is one entry, and these are its fields:

| Field | Meaning |
| --- | --- |
| `e` | A random id for the entry, 16 hex digits. A line whose `e` was seen before is a repeat. |
| `t` | When the change happened, in UTC (RFC 3339). |
| `host` | The machine that made it, with characters outside letters, digits, `_` and `-` replaced by `_`. |
| `id` | The session, or `inbox`. For `queue.move`, the queue the item left. |
| `op` | One of `queue.add`, `queue.rm`, `queue.edit`, `queue.move`, `queue.reorder`, `pin`, `unpin`, `snooze`, `unsnooze`, `alias`, `prune`, `session.start`, `session.end`. |
| `text` | The item, Objective or alias, at most 200 characters; for `queue.edit`, the new text. Left out when there is none. |
| `old` | `queue.edit`: the text before. `alias`: the name before. Left out when there is none. |
| `from`, `to` | `queue.reorder`: the positions the item left and took. Left out otherwise. |
| `dest` | `queue.move`: the queue the item went to. Left out otherwise. |

An item has no identity beyond its text, so follow one through an edit with `old` and `text`. New fields may be added, so a program should ignore the ones it does not know. To process the log, pipe `sitrep log --json` to `jq`, which handles repeats, torn lines, old entries and unsafe text that the raw files do not:

```
sitrep log --json -n 0 | jq -r 'select(.op=="queue.rm") | .text'
sitrep log --json --since 7d | jq -s 'group_by(.op) | map({op: .[0].op, n: length})'
ssh mbp sitrep log --json --since 2d | jq -s 'sort_by(.t)'
```
