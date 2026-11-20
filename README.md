# claude-sitrep

A situation report for each Claude Code session, plus a small per-session work queue. When several sessions run at once in separate terminals, the status line under each prompt says what the session is for, how far along it is, what it is doing right now and what is queued next.

## What You See

```
Objective: Build a three-line status block for my sessions
Progress:  Wrote the wrapper and hooks; summarizer tested against a fake transcript.
Now:       Waiting on a repo name.
Queue: 2 queued, next: update README  (session a1b2)
```

The block works as follows:

1. Objective starts as the first prompt of the session (a resumed session is seeded from its transcript) and is then replaced by a short title written by the same Haiku call that writes Progress and Now. An Objective you pin with `+goal` or `sitrep goal` is never rewritten.
2. Progress and Now are rewritten after each turn by a short Haiku call that reads the tail of the transcript.
3. Queue shows how many items are queued, the next item, and the first four characters of the session id (used to address the session from `sitrep`).

## Install

Requires `jq`, `perl` and the `claude` CLI. Run `./install.sh`. It backs up `~/.claude/settings.json`, adds `SessionStart`, `UserPromptSubmit` and `Stop` hooks, and points the status line at `statusline/statusline.sh`. Any existing status line, such as Claude HUD, is saved to `~/.claude/sitrep/sitrep.conf` as `SITREP_HUD_CMD` and runs first, so its output stays above the sitrep block. It also links `sitrep` into `~/bin` (`./install.sh --alias q` adds a short name, unless it is taken) and the `sitrep-queue` skill into `~/.claude/skills`, which lets Claude queue work and pin the Objective when you ask. Notes and config from earlier versions (`~/.claude/session-notes`, `~/.claude/sitrep.conf`) are moved into `~/.claude/sitrep/`. `./uninstall.sh` reverses all of this and leaves your notes in place.

Claude HUD is optional. With no existing status line, only the sitrep block is shown, and `install.sh` says so. If Claude HUD's `/claude-hud:setup` runs after sitrep is installed, it replaces the status line command and the sitrep lines disappear from the display, although the hooks and `sitrep` keep working. Re-run `./install.sh` to restore them: it saves the HUD command as the upstream again and puts the sitrep wrapper in front. If the upstream program is later removed, delete the `SITREP_HUD_CMD` line from `~/.claude/sitrep/sitrep.conf`.

## The Queue

`sitrep` addresses a session by any unique prefix of its id, as shown in the status line and in `sitrep ls`.

1. `sitrep ls` lists recent sessions with Objective, Now and queue length, and `sitrep ls <session>` lists one queue.
2. `sitrep add <session> <text>` appends an item.
3. `sitrep goal <session> <text>` pins that session's Objective.
4. `sitrep done <session> [n]` removes item `n` (default 1) and prints it.
5. `sitrep move <from> <to> [n]` hands an item to another session.

Inside a session, a prompt starting with `+ ` queues the rest of the line, and one starting with `+goal ` pins the Objective. Neither is sent to Claude. Both apply only to a single line of at most 200 characters; anything longer or multi-line goes to Claude as written, so a pasted list never gets swallowed.

## How It Works

State lives in `~/.claude/sitrep/<session_id>.{txt,queue,cwd}`, so the status line only reads files and renders instantly. The Haiku call runs in the background on `Stop`, is capped at $0.05 per call, and sets `CLAUDE_NOTES_RUNNING` so its own hooks do not recurse. At most one runs per 45 second window (`SITREP_DEBOUNCE`): a `Stop` inside the window schedules a single deferred summary for the end of it, and a summary that arrives after a newer prompt is dropped.

## Privacy And Safety

Prompt text is stored in `~/.claude/sitrep`, which is owner-only (`700`, files `600`). Prompt and model text is treated as untrusted terminal input, so control characters (escape sequences and the C1 range) and format characters (such as bidi overrides) are removed before it is stored and again before it is printed. The summarizer sends the tail of the transcript to Haiku through `claude -p`, the same service the session itself uses. It runs from an empty temporary directory with no tools, no MCP servers, no settings and a 60 second limit, and only reads transcripts that live under `~/.claude/projects`.

`~/.claude/sitrep/sitrep.conf` is sourced and run on every render, so keep it owner-only, as the installer does.

## Development

`make lint` runs `bash -n` and `shellcheck` over every script, and `make test` runs `tests/run.sh`, which needs bash, `jq`, `perl` and `tar`. The tests use a throwaway `$HOME` and a stub `claude`, so they never touch real settings or the API. Both run in CI on Linux and macOS. `lib/hooks.jq` holds the settings filter that `install.sh` and `uninstall.sh` share.

## Status

The queue does not feed work to Claude automatically. Whether a `Stop` hook should hand over the next item is TBD, as an unattended session that keeps itself busy is also a runaway session. Pushing the same text to the cmux sidebar has not been built; the notes are plain files, so a script can read them.

## License

MIT. See `LICENSE`.
