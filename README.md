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

1. Backs up `~/.claude/settings.json` and adds `SessionStart`, `UserPromptSubmit`, `Stop` and `SessionEnd` hooks.
2. Points the status line at `statusline/statusline.sh`. Any existing status line, such as Claude HUD, is saved to `~/.claude/sitrep/sitrep.conf` as `SITREP_HUD_CMD` and runs first, so its output stays above the sitrep block.
3. Links `sitrep` into `~/bin`. `./install.sh --alias q` adds a short alias, unless the name is taken.
4. Links the `sitrep-queue` skill into `~/.claude/skills`, which lets Claude queue work and pin the Objective when asked.
5. Moves state and config from earlier versions (`~/.claude/session-notes`, `~/.claude/sitrep.conf`) into `~/.claude/sitrep/`. A file that already exists there is never overwritten, so a stale copy can stay behind in `~/.claude/session-notes`. A folder holding files that are not ours is left alone.

`./uninstall.sh` reverses the hooks, status line and links, and leaves your state in place. After pulling an update, run `./install.sh` again: hook paths and names change between versions, and until it runs the old path in `settings.json` fails with a non-blocking "No such file" hook error. It also removes a dangling `~/bin/q` link left by the old command name.

Claude HUD is optional. With no existing status line, only the sitrep block is shown, and `install.sh` says so. If Claude HUD's `/claude-hud:setup` runs after sitrep is installed, it replaces the status line command and the sitrep lines disappear from the display, although the hooks and `sitrep` keep working. Re-run `./install.sh` to restore them: it saves the HUD command as the upstream again and puts the sitrep wrapper in front. If the upstream program is later removed, delete the `SITREP_HUD_CMD` line from `~/.claude/sitrep/sitrep.conf`.

## The Queue

`sitrep` addresses a session by any unique prefix of its id, as shown in the status line and in `sitrep ls`. `sitrep help` lists the commands, and `sitrep help <command>` or `sitrep <command> --help` shows the details of one.

1. `sitrep ls` lists running sessions with their queue (item count and next item), Objective and Now, and `sitrep ls <id>` lists one queue. `-a` adds ended sessions.
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

## The Dashboard

`sitrep watch [options] [seconds]` shows every running session at once, in the same shape as the status line, and is meant for a spare tmux pane. It is read-only. It redraws every N seconds (default 2), and straight away when a state file, the set of live sessions or the pane size changes. Press `q` or Ctrl-C to leave.

A session is running when a live process has its id in Claude Code's session registry (`~/.claude/sessions`), and `watch` and `ls` list only those. A session that is not registered but is under two minutes old stays listed, as the registry lags a start and a `/clear`. A session that has gone, or has said it ended, is hidden. The last lines of the frame say how many are hidden (`+2 ended (--all)`) and how many queue items they still hold (`3 items stranded in ended sessions (sitrep prune)`). `--all` or `-a` lists them with state `ended`. When the registry cannot be read, `watch` falls back to the hook states below, and a session counts as over once it has said `ended`.

Each session shows its id, directory, state and queue, then Objective, Progress and Now, newest first by when its state last changed. An `Inbox:` line sits on top when the inbox holds anything. The state comes from the hooks:

1. `working` from the moment a prompt goes to Claude.
2. `waiting` when Claude has stopped and it is your turn.
3. `ended` when the session exits.

The number after the state is how long ago it changed. Claude Code fires no hook when you interrupt a turn, so a `working` session whose transcript has been quiet for ten minutes shows as `idle?` with how long it has been quiet. A session whose terminal is closed or that is killed never fires `SessionEnd`, but its process is gone, so the registry hides it after two quiet minutes. A session with no state file, such as one that started before you upgraded, shows `-`.

Objective, Progress, Now and queue items wrap at the pane width with a hanging indent. The width is measured in display columns, so a wide character takes two. A field takes at most 3 lines and a queue item at most 2, and `…` ends the last line when text is cut. `--wrap <lines>` sets the field limit, and queue items then get 2 lines, or 1 when the limit is 1. When the sessions do not fit the pane height, whole sessions are dropped and the last line says `+N more`.

Both `watch` and `ls -v` list each session's queue, and the inbox's, under the session, at most 5 items each, then `+N more` if the queue is longer. `-n <items>` changes the limit for one run, and `-n 0` returns to the compact form, with the count and the next item in the header. `sitrep ls -v` prints the same frame as `ls` with a Progress line added and no wrapping.

In a terminal, a session whose state changed in the last 60 seconds has a background colour on all its lines. `--highlight <seconds>` changes the window, and `0` turns it off. A pipe, `--once` and `NO_COLOR` never get colour. When the output is not a terminal, or `TERM` is unset or `dumb`, `sitrep watch` prints one frame and exits, as `sitrep watch --once` does.

Defaults live in `~/.claude/sitrep/sitrep.conf` or the environment, and the environment wins:

1. `SITREP_QUEUE_ITEMS=<items>` is the queue list limit (default 5).
2. `SITREP_HIGHLIGHT_SECONDS=<seconds>` is the highlight window (default 60).
3. `SITREP_HIGHLIGHT_SGR=<codes>` is the highlight colour as terminal SGR codes (default `48;5;238`, a dark grey). Only digits and semicolons are accepted.

`sitrep` reads `sitrep.conf` for these lines and does not run it, and the installers replace or remove only their own `SITREP_HUD_CMD` line in it.

## Pruning

`sitrep prune [-y] [--dry-run]` tidies sessions that are gone. It works as follows:

1. It reads the live-session registry, and changes nothing if it cannot.
2. It marks each session that is not running as `ended`.
3. For a session that still holds queue items, it asks `Move N item(s) from <id> (<directory>) to the inbox? [y/N]`. `-y` answers yes for all. Without a terminal and without `-y` the items stay where they are, and it says so.
4. `--dry-run` prints what it would do and changes nothing.

It never touches a running session or one under two minutes old, and it never deletes a queue file.

## How It Works

State lives in `~/.claude/sitrep/`: `<session_id>.{txt,queue,cwd}` per session, `inbox.queue` for the inbox, hidden `.<session_id>.*` files for the pin, the prompt count, the debounce and the session state, and `sitrep.conf`. The status line only reads files, so it renders instantly. The Haiku call runs in the background on `Stop`, is capped at $0.05 per call, and sets `SITREP_SUMMARIZING` so its own hooks do not recurse. At most one runs per 45 second window (`SITREP_DEBOUNCE`). A `Stop` inside the window schedules one deferred run for its end, and a result that arrives after a newer prompt is dropped.

## Privacy And Safety

Session state is stored in `~/.claude/sitrep`, which is owner-only (`700`, files `600`). Prompt and model text is treated as untrusted terminal input, so control characters (escape sequences and the C1 range) and format characters (such as bidi overrides) are removed before it is stored and again before it is printed. The summarizer sends the tail of the transcript to Haiku through `claude -p`, the same service the session itself uses. It runs from an empty temporary directory with no tools, no MCP servers, no settings and a 60 second limit, and only reads transcripts that live under `~/.claude/projects`.

`~/.claude/sitrep/sitrep.conf` is sourced and run on every render, so keep it owner-only, as the installer does.

## Development

`make lint` runs `bash -n` and `shellcheck` over every script, and `make test` runs `tests/run.sh`, which needs bash, `jq`, `perl` and `tar`. The tests use a throwaway `$HOME` and a stub `claude`, so they never touch real settings or the API. Both run in CI on Linux and macOS. `lib/hooks.jq` holds the settings filter that `install.sh` and `uninstall.sh` share.

## Status

The queue does not feed work to Claude automatically. Whether a `Stop` hook should hand over the next item is TBD, as an unattended session that keeps itself busy is also a runaway session. Pushing the same text to the cmux sidebar has not been built; the state is plain files, so a script can read it.

## License

MIT. See `LICENSE`.
