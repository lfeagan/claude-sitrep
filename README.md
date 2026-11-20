# claude-sitrep

A situation report for each Claude Code session, plus a small per-session work queue. When several sessions run at once in separate terminals, the status line under each prompt says what the session is for, how far along it is, what it is doing right now and what is queued next.

## What You See

```
Objective: Build a three-line status block for my sessions
Progress:  Wrote the wrapper and hooks; summarizer tested against a fake transcript.
Now:       Waiting on a repo name.
Queue (2) [a1b2]  next: update README
```

The four lines work as follows:

1. Objective is set once from the first prompt of the session and then kept. A resumed session is seeded from its transcript at start, so it does not wait for a new prompt.
2. Progress and Now are rewritten after each turn by a short Haiku call that reads the tail of the transcript.
3. Queue shows the item count, the first four characters of the session id (used to address the session from `q`) and the next item.

## Install

Requires `jq` and the `claude` CLI. Run `./install.sh`. It backs up `~/.claude/settings.json`, adds `SessionStart`, `UserPromptSubmit` and `Stop` hooks, and points the status line at `statusline/statusline.sh`. Any existing status line, such as Claude HUD, is saved to `~/.claude/sitrep.conf` as `SITREP_HUD_CMD` and runs first, so its output stays above the sitrep block. `./uninstall.sh` reverses all of this.

## The Queue

`q` addresses a session by any unique prefix of its id, as shown in the status line and in `q ls`.

1. `q ls` lists recent sessions with Objective, Now and queue length, and `q ls <session>` lists one queue.
2. `q add <session> <text>` appends an item.
3. `q done <session> [n]` removes item `n` (default 1) and prints it.
4. `q move <from> <to> [n]` hands an item to another session.

5. `q goal <session> <text>` pins that session's Objective.

Inside a session, a prompt starting with `+ ` queues the rest of the line, and one starting with `+goal ` pins the Objective. Neither is sent to Claude.

## How It Works

State lives in `~/.claude/session-notes/<session_id>.{txt,queue,cwd}`, so the status line only reads files and renders instantly. The Haiku call runs in the background on `Stop`, is skipped if the last update was under 45 seconds ago, is capped at $0.05 per call, and sets `CLAUDE_NOTES_RUNNING` so its own hooks do not recurse.

## Privacy And Safety

Prompt text is stored in `~/.claude/session-notes`, which is owner-only (`700`, files `600`). Prompt and model text is treated as untrusted terminal input, so control characters such as escape sequences are removed before it is stored and again before it is printed. The summarizer sends the tail of the transcript to Haiku through `claude -p`, the same service the session itself uses.

## Development

`make lint` runs `bash -n` and `shellcheck` over every script, and `make test` runs `tests/run.sh`, which needs only bash and `jq`. The tests use a throwaway `$HOME` and a stub `claude`, so they never touch real settings or the API. Both run in CI on Linux and macOS.

## Status

The queue does not feed work to Claude automatically. Whether a `Stop` hook should hand over the next item is TBD, as an unattended session that keeps itself busy is also a runaway session. Nothing is pushed to the cmux sidebar yet.
