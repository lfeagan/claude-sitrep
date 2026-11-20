# claude-sitrep

A situation report for each Claude Code session, plus a small per-session work queue. When several sessions run at once in separate terminals, the status line under each prompt says what the session is for, how far along it is, what it is doing right now and what is queued next.

## What You See

```
Objective: Build a three-line status block for my sessions
Progress:  Wrote the wrapper and hooks; summarizer tested against a fake transcript.
Now:       Waiting on a repo name.
Queue: 2 items, next: update README  [id a1b2]
```

The block works as follows:

1. Objective starts as the first prompt (a resumed session is seeded from its transcript), so the raw prompt shows until the first turn finishes. A short title written by the same Haiku call that writes Progress and Now then replaces it. A pinned Objective (`+pin` or `sitrep pin`) stays until you run `sitrep unpin`.
2. Progress and Now are rewritten after each turn by a short Haiku call that reads the tail of the transcript.
3. Queue shows how many items are waiting, the next item and the first four characters of the session id, which is how `sitrep` addresses the session (`sitrep add a1b2 ...`).

## Install

Requires `jq`, `perl` and the `claude` CLI. Run `./install.sh`, which does the following:

1. Backs up `~/.claude/settings.json` and adds `SessionStart`, `UserPromptSubmit` and `Stop` hooks.
2. Points the status line at `statusline/statusline.sh`. Any existing status line, such as Claude HUD, is saved to `~/.claude/sitrep/sitrep.conf` as `SITREP_HUD_CMD` and runs first, so its output stays above the sitrep block.
3. Links `sitrep` into `~/bin`. `./install.sh --alias q` adds a short alias, unless the name is taken.
4. Links the `sitrep-queue` skill into `~/.claude/skills`, which lets Claude queue work and pin the Objective when asked.
5. Moves state and config from earlier versions (`~/.claude/session-notes`, `~/.claude/sitrep.conf`) into `~/.claude/sitrep/`. A file that already exists there is never overwritten, so a stale copy can stay behind in `~/.claude/session-notes`. A folder holding files that are not ours is left alone.

`./uninstall.sh` reverses the hooks, status line and links, and leaves your state in place. After pulling an update, run `./install.sh` again: hook paths and names change between versions, and until it runs the old path in `settings.json` fails with a non-blocking "No such file" hook error. It also removes a dangling `~/bin/q` link left by the old command name.

Claude HUD is optional. With no existing status line, only the sitrep block is shown, and `install.sh` says so. If Claude HUD's `/claude-hud:setup` runs after sitrep is installed, it replaces the status line command and the sitrep lines disappear from the display, although the hooks and `sitrep` keep working. Re-run `./install.sh` to restore them: it saves the HUD command as the upstream again and puts the sitrep wrapper in front. If the upstream program is later removed, delete the `SITREP_HUD_CMD` line from `~/.claude/sitrep/sitrep.conf`.

## The Queue

`sitrep` addresses a session by any unique prefix of its id, as shown in the status line and in `sitrep ls`.

1. `sitrep ls` lists sessions active in the last day with its queue (item count and next item), Objective and Now, and `sitrep ls <id>` lists one queue.
2. `sitrep add <id> <text>` appends an item.
3. `sitrep pin <id> <text>` pins that session's Objective, and `sitrep unpin <id>` lets the summarizer rewrite it again.
4. `sitrep rm <id> [n]` removes item `n` (default 1) and prints it.
5. `sitrep move <from> <to> [n]` hands an item to another queue.
6. `sitrep top <id> <n>` moves item `n` to the front, and `sitrep reorder <id> <from> <to>` moves an item to any position. The status line and `sitrep ls` show the first item as the next one.

The inbox is a queue that belongs to no session, for work you have not yet matched to one. Address it as `inbox` in any command that takes an id: `sitrep add inbox <text>`, `sitrep ls inbox`, `sitrep rm inbox [n]`, and `sitrep move inbox <id>` when you pick the item up. It has no Objective, so `pin` and `unpin` do not apply. `sitrep ls` prints an `Inbox:` line above the table when it holds anything, and the status line appends `Inbox: N` to the Queue line. From inside a Claude Code session, `+inbox <text>` adds to it and `+take` pulls from it (see below). The inbox is one file that every session can append to, so two commands that rewrite it in the same instant can lose an append. `rm` and `move` refuse to delete an item that changed after they read it, but an `add` that lands while a `move` is rewriting the source queue can still be lost.

Inside a session, four prefixes work on the prompt itself, and none of them is sent to Claude:

1. `+ <text>` queues the text to this session.
2. `+pin <text>` pins the Objective.
3. `+inbox <text>` queues the text to the inbox.
4. `+take [n]` moves item `n` of the inbox (default 1) into this session's queue.

The text forms apply only to a single line of at most 200 characters, and `+take` accepts only an optional number. Anything else goes to Claude as written, so a pasted list, or a prompt such as "+take care of the rest", never gets swallowed. So does a prefix with a leading space, and a `+take` that cannot succeed, such as one on an empty inbox.

## How It Works

State lives in `~/.claude/sitrep/`: `<session_id>.{txt,queue,cwd}` per session, `inbox.queue` for the inbox, hidden `.<session_id>.*` files for the pin, the prompt count and the debounce, and `sitrep.conf`. The status line only reads files, so it renders instantly. The Haiku call runs in the background on `Stop`, is capped at $0.05 per call, and sets `SITREP_SUMMARIZING` so its own hooks do not recurse. At most one runs per 45 second window (`SITREP_DEBOUNCE`). A `Stop` inside the window schedules one deferred run for its end, and a result that arrives after a newer prompt is dropped.

## Privacy And Safety

Session state is stored in `~/.claude/sitrep`, which is owner-only (`700`, files `600`). Prompt and model text is treated as untrusted terminal input, so control characters (escape sequences and the C1 range) and format characters (such as bidi overrides) are removed before it is stored and again before it is printed. The summarizer sends the tail of the transcript to Haiku through `claude -p`, the same service the session itself uses. It runs from an empty temporary directory with no tools, no MCP servers, no settings and a 60 second limit, and only reads transcripts that live under `~/.claude/projects`.

`~/.claude/sitrep/sitrep.conf` is sourced and run on every render, so keep it owner-only, as the installer does.

## Development

`make lint` runs `bash -n` and `shellcheck` over every script, and `make test` runs `tests/run.sh`, which needs bash, `jq`, `perl` and `tar`. The tests use a throwaway `$HOME` and a stub `claude`, so they never touch real settings or the API. Both run in CI on Linux and macOS. `lib/hooks.jq` holds the settings filter that `install.sh` and `uninstall.sh` share.

## Status

The queue does not feed work to Claude automatically. Whether a `Stop` hook should hand over the next item is TBD, as an unattended session that keeps itself busy is also a runaway session. Pushing the same text to the cmux sidebar has not been built; the state is plain files, so a script can read it.

## License

MIT. See `LICENSE`.
