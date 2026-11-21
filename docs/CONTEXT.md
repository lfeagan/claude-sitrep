# Context window column, version 0.8.0

Status: design, reviewed once. Changes from the review: the percentage is not taken from `used_percentage` when it is 0 (Claude Code sends 0 or null before the first answer), `at` means last seen, a record is made only for a session that has notes, the claim that the write cannot delay the line is gone, a low value is marked with `!` and not a colour inside a coloured row, the column drops by the room NOW has left, the sort and the Watch line are a second round, and `left_percent` is not in the JSON.

## Purpose

The Top tab shows what each session is doing but not how full its context window is, which is what decides whether a session needs a `/compact` or a fresh start. This document adds a CTX column that shows the share of the window left, and a `context` field in `sitrep ls --json` that carries the numbers.

## Where The Numbers Come From

1. The transcript has the tokens of the last turn (`usage` on the last assistant message: `input_tokens` plus `cache_creation_input_tokens` plus `cache_read_input_tokens`) but not the size of the window, which is 200,000 or 1,000,000 tokens by model and plan and which no transcript field names. A guess from the model name is wrong for a session on the larger window that is still below 200,000 tokens, which then shows as fuller than it is.
2. The status line's input has both: Claude Code hands every status line command `context_window.used_percentage`, `context_window.context_window_size` and `context_window.current_usage`. `sitrep statusline` already reads that input for the session id, so it can record the numbers where the dashboard can read them.

The recommendation is the status line, as the exact source. Sessions that never draw a status line (background sessions, `claude -p`) show `-` until a later round reads the transcript for them.

## The Record

1. `sitrep statusline` writes `sessions/<id>/context` when the input carries a window size: one line, `<used percent> <window size> <tokens> <epoch seconds>`, for example `37.5 1000000 375000 1800000000`. It is made the way every state file is (`Store.WriteFile`, `0600`, a unique temporary name).
2. It records only for a session that has a `notes` file already (`state.IsRegular`), and only after the session id has passed the check that rejects the inbox and an invalid id. An unknown id therefore makes no folder, and a state folder that is missing or read-only means no record.
3. The tokens are `input_tokens`, `cache_creation_input_tokens` and `cache_read_input_tokens` of `current_usage`, which leaves out the output of the last turn (a small undercount). The percentage is `used_percentage` when it is a number above 0. Claude Code sends 0 or null before the first answer even though `current_usage` is real, so otherwise it is tokens over window size. With a window size and no `current_usage` (a new session, or just after `/compact`) the session is recorded as 0 used. The document does not promise that the percentage and the tokens agree: the status line's own numbers do not always.
4. A percentage under 0 or over 100, a size under 1 or over 100,000,000, a negative number of tokens and anything that is not a number record nothing.
5. The file is rewritten when its numbers differ, and when its time is more than five minutes old, so `at` means last seen: a live session with a steady value stays fresh, and an idle one ages. Nothing is greyed or dropped for age in the first round, as context does not shrink while a session is idle.
6. Recording is best effort and comes last. Claude Code gets the line when the process exits, so the write is one small file written before that, and any error is dropped. The README and the help, which say the status line only reads files, say that it also records the context window.
7. A reader (`Store.Context(id)`, the only one) takes the file through `state.ReadFile` with a cap of 128 bytes, and accepts only four numbers in range. A link, a pipe, a large file or anything else is no record.

## The Snapshot And The JSON

1. `snapshot.Session` gains `Context *Context` after `Queue`, which is `nil` (`null` in the JSON) without a record or for an ended session: `{"used_percent": 37.5, "window": 1000000, "tokens": 375000, "at": 1800000000}`. The share left is `100 - used_percent`, and the column rounds it down.
2. `ls --json` stays version 1, as a new field needs no new version. The plain text of `ls` does not change. The differential JSON test strips `"context": null` from the output of the Go program, as it does for `alias`, since no scenario of it sets a window.
3. The dashboard has its own `Session` type and its own copy of the snapshot (`FromSnapshot`, `sanitize`): both carry the field, and `sanitize` clamps the percentage to 0 to 100 (and a NaN to nothing) and the window to at least 1.

## The Column

1. The Top tab gains a CTX column after QUEUE, four wide: the percentage left, rounded down, as `62%`, and `-` where there is no record. At 10% or less the value is marked with a leading `!` (`!9%`), so the plain text carries the warning too. At 20% or less it is bold, on a row that is not already coloured as needing the user or as recently active, as a colour reset inside such a row would end its colour.
2. The column is dropped when the NOW text would get fewer than 20 columns, computed from the widths of the row, so a narrow screen loses it before it loses the line that says what a session is doing. The cap on the directory column takes the six columns into account when CTX is shown.

## Second Round

The Top sort by the least context left (sessions with no record last, never as 0% left), `ctx 62%` on the first line of a Watch block after the queue text, the transcript for sessions with no status line, and any greying of old values.

## What Does Not Change

The hooks, the summarizer, the queue and `migrate` do not read the file, and `prune` leaves it with the rest of the folder. A session of 0.7.x has no file and shows `-` until its status line draws once under 0.8.0.

## Tests

1. The record: a window in the input writes the file, the same numbers do not rewrite it, new numbers do, and so does an old time; a percentage of 0 or null with a real `current_usage` records tokens over size; a window and no usage records 0 used; an out-of-range value, a window of 0 and text instead of numbers record nothing; an unknown session id makes no folder; the mode is `0600`; no temporary file is left; a read-only or missing folder does not stop the line from being drawn.
2. The reader: a hostile file (a link, a pipe, a megabyte, escape characters, a negative number, `NaN`, extra fields) is no record.
3. The snapshot and the JSON: the field, `null` for an ended session, the dashboard's copy and its clamping, and the differential suite, which keeps passing with the normaliser. The status line's differential scenarios carry no `context_window`, so they write nothing.
4. The column: the values, `-`, the marker and the bold at 10% and 20%, no colour inside a coloured row, the drop by room at widths 59, 60, 80 and 120 and no line wider than the screen, and the header.

## Rollback Cost

A user who goes back to 0.7.x keeps a file that the old program ignores. Nothing else needs undoing.

## Open Questions

1. Should the transcript fill in for sessions with no status line? It costs a read of the end of each transcript on each refresh unless the result is cached by the file's size and time, and the window size is a guess there.
2. Claude Code compacts on its own near the limit, and a user may set the window it compacts at, so "left" is relative to the window and not to the compaction point. The status line's `used_percentage` is taken as it is, and the document does not correct it.
