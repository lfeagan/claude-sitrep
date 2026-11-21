# claude-sitrep

A situation report for each Claude Code session, plus a small per-session work queue. When several sessions run at once in separate terminals, the status line under each prompt says what the session is for, how far along it is, what it is doing right now and what is queued next.

## What You See

```
Objective: Build a three-line status block for my sessions
Progress:  Wrote the wrapper and hooks; summarizer tested against a fake transcript.
Now:       Waiting on a repo name.
Queue [id a1b2]: 2 items, next: update README
```

The block works as follows:

1. Objective starts as the first prompt (a resumed session is seeded from its transcript), so the raw prompt shows until the first turn finishes. A short title written by the same Haiku call that writes Progress and Now then replaces it. A pinned Objective (`+pin` or `sitrep pin`) stays until you run `sitrep unpin`.
2. Progress and Now are rewritten after each turn by a short Haiku call that reads the tail of the transcript.
3. Queue shows how many items are waiting, the next item and the first four characters of the session id, which is how `sitrep` addresses the session (`sitrep add a1b2 ...`).

## Why sitrep

Most tools show what your sessions are doing. sitrep also holds what you plan to do next, per session, and uses plain files and one small program, so the status line renders instantly and there is no server to run.

1. A status block and a per-session queue under each prompt, plus an inbox for work you have not yet matched to a session. `+ <text>` queues an item without sending anything to Claude.
2. Objective, Progress and Now come from a short Haiku call after each turn. See Privacy And Safety for the cost cap and what is sent.
3. A full-screen dashboard (`sitrep watch`) and a stable JSON format (`sitrep ls --json`) for your own tools.
4. Limits: Claude Code only, one machine, no cost tracking, and it cannot answer a session's approval prompts.

`docs/COMPARISON.md` compares it with Agent View, claudectl, Plate-Spinner and the kanban boards, and says when one of them is the better choice.

## Install

sitrep is one Go program. It needs Go 1.26 or newer to build, and the `claude` CLI.

```
/plugin install sitrep --marketplace lfeagan/claude-sitrep
/sitrep:setup
```

The first line adds this repository as a marketplace and installs the plugin, which brings the program's source, the hooks and the `sitrep-queue` skill (as `/sitrep:sitrep-queue`). A plugin cannot build a program or declare a status line, so `/sitrep:setup` does the rest, after Claude Code asks about the settings change:

1. It builds the program from the plugin's own folder into `~/.claude/sitrep/bin/sitrep`, which the hooks and the status line run, so it always matches the plugin.
2. It points the status line at `"$HOME/.claude/sitrep/bin/sitrep" statusline`. An existing status line, such as Claude HUD, is saved as `SITREP_HUD_CMD` in `~/.claude/sitrep/sitrep.conf` and runs first.
3. It links `sitrep` into `~/bin` (`BIN_DIR` changes the folder), unless something else is there.

`sitrep setup` backs up `~/.claude/settings.json` (to `settings.json.bak.<time>`, owner-only, never over an earlier backup) and checks the edit before it writes anything. It changes only the `statusLine` key and leaves the rest of the file as it was, a symlinked file included. If it finds an install from a checkout of the old shell version, it stops, changes nothing, and names what to take out by hand.

sitrep runs on macOS and Linux; it uses file locks and process groups, so there is no Windows build.

### Updating

When a newer release exists, the status line says so under the queue line (`Update: sitrep 0.6.0 is out (you have 0.5.0)`). Updating is two steps, because the hooks and the status line run the built program, and a plugin update does not rebuild it:

1. Update the plugin with `claude plugin marketplace update sitrep && claude plugin update sitrep@sitrep`, or from `/plugin` inside Claude Code. Restart Claude Code to apply it.
2. Run `/sitrep:setup` again, so that the program matches the new plugin. It is safe to run again. `sitrep version` prints the version of the program, and `.claude-plugin/plugin.json` the version of the plugin.

Between the two steps the hooks run the older program, and if you skipped step 2 after the first install they name a program that does not exist yet, so each hook ends with a non-blocking error and a `+` prompt goes to Claude as written.

From a clone, with no plugin: `git pull --tags && make install`. `make install` builds the program and runs its `setup`, as `/sitrep:setup` does. `go install github.com/lfeagan/claude-sitrep/cmd/sitrep@latest` builds the same program from the newest version tag, and `sitrep setup` then puts it in place, wraps the status line and links the command; the hooks come only with the plugin.

Once a day a hook starts `sitrep update-check`, which runs `git ls-remote --tags` against this repository and records the newest `vX.Y.Z` tag in `~/.claude/sitrep/update`. It downloads and installs nothing, and the status line only reads that file. It needs `git` and the network, and fails quietly without them. `SITREP_CHECK_UPDATES=0` turns it off.

### Upgrading from 0.3.x

The shell versions left a status line, hooks or links that setup will not run beside. Setup stops, changes nothing, and names them. For the 0.3.x plugin setup that is the status line `bash "$HOME/.claude/sitrep/statusline.sh" # claude-sitrep`: put back the status line you had before (the command saved in `~/.claude/sitrep/sitrep.conf` as `SITREP_HUD_CMD`), or delete the `statusLine` key, and run `/sitrep:setup` again. For an install from a checkout (`./install.sh`), remove its four hooks and its status line from `~/.claude/settings.json` the same way, and delete the links `~/bin/sitrep` and `~/.claude/skills/sitrep-queue`. State is kept, and its files have the same names.

To take sitrep out, run `/sitrep:setup remove` (it restores the saved upstream status line and removes the program and its links), then `/plugin uninstall sitrep`. State in `~/.claude/sitrep` stays. If Claude HUD's setup runs after sitrep's, it replaces the status line and the sitrep block disappears: run `/sitrep:setup` again.

## The Queue

`sitrep` addresses a session by any unique prefix of its id, as shown in the status line and in `sitrep ls`. `sitrep help` lists the commands, and `sitrep help <command>` or `sitrep <command> --help` shows the details of one.

1. `sitrep ls` lists running sessions with their queue (item count and next item), Objective and Now, and `sitrep ls <id>` lists one queue. `-a` adds ended sessions.
2. `sitrep add <id> <text>` appends an item.
3. `sitrep pin <id> <text>` pins that session's Objective, and `sitrep unpin <id>` lets the summarizer rewrite it again.
4. `sitrep rm <id> [n]` removes item `n` (default 1) and prints it.
5. `sitrep move <from> <to> [n]` hands an item to another queue. With `--item <text>` it refuses unless item `n` is exactly that text, so a program that showed a list cannot move a different item. `rm`, `top` and `reorder` take the same `--item <text>`, which is checked against the item at `n` (or at `from`).
6. `sitrep edit <id> <n> <text>` replaces the text of item `n` (a spelling fix, say) and keeps its place; it takes the same `--item <old text>` guard.
7. `sitrep alias <id> <name>` gives a session a name, to use wherever an id goes (`sitrep ls sitrep`), and `sitrep alias <id> --off` takes it away; with no arguments it lists the names. A name is 1 to 32 lowercase letters, digits and hyphens, starts with a letter, and is neither `inbox` nor the start of a session id. A name that another session holds is refused, so free it first. An id always wins over a name, and a name must match whole (`sit` does not find `sitrep`). `sitrep ls` and the dashboard show the name in the first column in place of the four characters, and the status line shows `Queue [id c5d0 sitrep]`.
8. `sitrep top <id> <n>` moves item `n` to the front, and `sitrep reorder <id> <from> <to>` moves an item to any position. The status line and `sitrep ls` show the first item as the next one.

Changes to a queue take a lock, so an `add` that lands while a `move` or a `reorder` rewrites the file waits for it, and nothing is lost.

The inbox is a queue that belongs to no session, for work you have not yet matched to one. Address it as `inbox` in any command that takes an id: `sitrep add inbox <text>`, `sitrep ls inbox`, `sitrep rm inbox [n]`, and `sitrep move inbox <id>` when you pick the item up. It has no Objective, so `pin` and `unpin` do not apply. `sitrep ls` prints an `Inbox:` line above the table when it holds anything, and the status line appends `Inbox: N` to the Queue line.

Inside a session, four prefixes work on the prompt itself, and none of them is sent to Claude:

1. `+ <text>` queues the text to this session.
2. `+pin <text>` pins the Objective.
3. `+inbox <text>` queues the text to the inbox.
4. `+take [n]` moves item `n` of the inbox (default 1) into this session's queue.

The text forms apply only to a single line of at most 200 characters, and `+take` accepts only an optional number. Anything else goes to Claude as written, so a pasted list, or a prompt such as "+take care of the rest", never gets swallowed. So does a prefix with a leading space, and a `+take` that cannot succeed, such as one on an empty inbox.

## Shell Completion

`eval "$(sitrep completion bash)"` in `~/.bashrc` completes command names, flags, the values of `--tab`, and session ids: the shortest prefix of at least four characters that no other session shares, which is how the status line shows them, and `inbox` where the command takes it. The script asks the program itself what to offer, so it cannot go out of date, and it works in the bash 3.2 that macOS ships.

## Listing Sessions

`sitrep ls` prints a table of the running sessions: id, directory, state and queue, then Objective and Now, newest first by when the state last changed. `-v` adds the Progress line and lists each queue (and the inbox) under its session, at most 5 items each and then `+N more`. `-n <items>` changes the limit for one run and implies `-v`, and `-n 0` returns to the compact form. `SITREP_QUEUE_ITEMS` sets the default.

A session is running when a live process has its id in Claude Code's session registry (`~/.claude/sessions`). A session that is not registered but is under two minutes old stays listed, as the registry lags a start and a `/clear`. A session that has gone, or has said it ended, is hidden. The last lines say how many are hidden (`+2 ended (--all)`) and how many queue items they still hold (`3 items stranded in ended sessions (sitrep prune)`). When the registry cannot be read, a session counts as over once it has said `ended`.

The state comes from the hooks:

1. `working` from the moment a prompt goes to Claude.
2. `waiting` when Claude has stopped and it is your turn.
3. `ended` when the session exits.

The number after the state is how long ago it changed. Claude Code fires no hook when you interrupt a turn, so a `working` session whose transcript has been quiet for ten minutes shows as `idle?` with how long it has been quiet. A session with no state file shows `-`.

## Pruning

`sitrep prune [-y | -k | --dry-run]` tidies sessions that are gone. It works as follows:

1. It reads the live-session registry, and changes nothing if it cannot.
2. It marks each session that is not running as `ended`.
3. For a session that still holds queue items, it asks `Move N item(s) from <id> (<directory>) to the inbox? [y/N]`. `-y` answers yes for all. Without a terminal and without `-y` the items stay where they are, and it says so.
4. `--dry-run` prints what it would do and changes nothing, and `-k` or `--keep` marks the sessions ended and leaves every queue alone without asking, for a program that has no one to answer. The three options exclude one another.

It never touches a running session or one under two minutes old, and it never deletes a queue file.

## The Dashboard

`sitrep watch` is a full-screen view with two tabs and a status bar, written with Bubble Tea and Lip Gloss and built into the same program. Without a terminal (a pipe, `TERM` unset or `dumb`) it prints one frame and exits, and `sitrep status` always does: `sitrep status [--tab watch|top] [--width n] [-a]`.

The two tabs are:

1. **Watch** shows Objective, Progress, Now and the queue of each session, wrapped with a hanging indent.
2. **Top** is a summary line (`3 running: 1 working, 1 waiting, 1 idle?, 2 ended`) and one line per session, with its state, queue size and what it is doing now.

A session that waits for you, meaning Claude has stopped and the next move is yours, is drawn with a soft pink-red background and a `●` before its state, until you send it a prompt. The status bar then leads with `● 2 need you`. The rule is the `needs_input` field of `sitrep ls --json`.

Keys:

1. `up`, `down`, `j` and `k` move the selection, which follows its session through refreshes and re-sorts, `gg` and `G` (and `home` and `end`) select the first and last, and `ctrl+d`, `ctrl+u`, `pgdn` and `pgup` move by a page. A lone `g` waits for a second one, shown as `g-` in the status bar; any other key cancels it, and does nothing else, as in vim.
2. `enter` jumps to the terminal that shows the selected session, when the dashboard itself runs inside tmux or cmux. It finds it through the session's process id, and through the `claude attach` client for a session that runs in the background. In tmux it selects the window, pane and client by tmux's own ids. In cmux it reads `cmux top --all --processes` to find the surface that holds the process, and focuses its window, workspace, pane and surface. If it cannot find one it says so and changes nothing. `--quit-on-jump` leaves the dashboard after a good jump, for a tmux popup.
3. `d` or the space bar opens the selected session in full, with nothing cut off and its whole queue. `q`, `esc` or `d` closes it, `up` and `down` show the neighbouring sessions, and `pgup`, `pgdn`, `ctrl+d`, `ctrl+u`, the space bar and `b` scroll, with `gg` and `G` at the top and the bottom.
4. `tab`, `shift+tab`, `h`, `l`, `left`, `right`, `1` and `2` switch tabs, `a` shows or hides ended sessions, and `s` cycles the Top sort through activity, state, directory and queue size.
5. `/` filters the sessions by a case-insensitive match in their id, directory, state, Objective, Progress and Now. What you type is text, `enter` keeps the filter and `esc` clears it.
6. `y` copies the selected session's id and `Y` its `claude --resume <id>` command, through the terminal's clipboard escape (OSC 52); the status bar shows the text as well. An id with unusual characters is never copied.
7. `i` opens the inbox and `e` the selected session's queue, as one list. In a queue, `o` (or `a`) types a new item, `enter` (or `E`) edits the selected one in place (the line starts with its text, and `esc` cancels), `x` removes the selected one after a `y` or `n`, `K` and `J` move it up and down, `T` moves it to the front, `P` types the Objective to pin and `U` unpins it. In the inbox, `t` takes the selected item into the session that was selected when you pressed `i`, after a `y` or `n`. The keys that act on an item pass its text, so they refuse, and say so, if the queue changed since the list was drawn.
8. `n` names the selected session: a line opens in the status bar with its current name, `enter` sets it, an empty line clears it, and `esc` cancels. It runs `sitrep alias`, so the same rules apply, and an ended session can be named too.
9. `z` snoozes the selected session: it is drawn with a hollow `○` and no colour, and drops out of the count of those that need you, until it changes by itself (the next prompt, or the end of the next turn). `z` again ends it at once.
10. `p` prunes the sessions that are gone: it shows what `sitrep prune --dry-run` would do and asks. `y` moves their queue items to the inbox, `n` leaves the items, and `esc` cancels. It will not run without the live-session registry.
11. `r` refreshes, and `?` lists the keys and colours from every screen (any key closes the list, and the screen under it is as it was). `q` goes back from the detail view, the queue and the prune screen, and quits from the tabs; `ctrl+c` quits from anywhere.

The terminal bell rings once when a session starts needing you. It stays quiet on the first load and for a snoozed session. `--no-bell` or `SITREP_TUI_BELL=0` turns it off. `--read-only` or `SITREP_TUI_READONLY` turns off every key that changes state. `--interval` sets the refresh (default 3s), and `--tab top` opens on the Top tab.

The dashboard reads the same snapshot as `sitrep ls --json`, and changes state only by running the same fixed commands you can run yourself (`prune`, `snooze` and the queue edits), in the same process, after a preview or a confirm. If a refresh fails, the last good screen stays and the status bar says why. It strips control and format characters from everything it draws.

## Settings

Defaults live in `~/.claude/sitrep/sitrep.conf` or the environment, and the environment wins:

1. `SITREP_QUEUE_ITEMS=<items>` is the queue list limit (default 5).
2. `SITREP_HIGHLIGHT_SECONDS=<seconds>` is how long a session counts as recently active, for the dashboard's colour (default 60, and 0 turns it off).
3. `SITREP_HIGHLIGHT_SGR=<codes>` is that colour as terminal SGR codes (default `48;5;238`, a dark grey). Only digits and semicolons are accepted.
4. `SITREP_DEBOUNCE=<seconds>` is the time between summaries (default 45; a value that is not a number turns the debounce off).
5. `SITREP_CHECK_UPDATES=<0|1>` lets sitrep look once a day for a newer release (default on; 0, false and no turn it off). See Updating.
6. `SITREP_THEME=<name>` picks the dashboard's colours: `default`, `high-contrast` (stronger colours, for a bright room or a poor screen) or `mono` (no colour at all: bold for recent activity, reverse video for what needs you and for the bars). A name it does not know gives the default.
7. `SITREP_COLORS=<name=colour,...>` changes single colours of the theme: `accent` (the tabs, the rule, the selection bar and the status bar), `dim` (muted text), `hot` (the background of recent activity), `attn` (the background of a session that needs you) and `error`. A colour is `#rrggbb` or a terminal colour number from 0 to 255, and `light/dark` gives one value for each kind of terminal, as in `SITREP_COLORS='attn=#ffc0c0/#601020,accent=#d9480f'`. The text on the accent colour is chosen to be readable. An entry it cannot read is ignored, and the others still apply. These win over the theme and over `SITREP_HIGHLIGHT_SGR`, which keeps working for the highlight.
8. `SITREP_TUI_INTERVAL=<seconds or duration>` is how often `sitrep watch` refreshes (default 3 seconds; `5`, `5s`, `1500ms` and `1m` are read, nothing below one second or above a day). `--interval` wins over it.

`sitrep.conf` holds `NAME=value` lines, with single quotes, double quotes and backslashes understood. It is read and never run, so it can hold no commands, and setup replaces or removes only its own `SITREP_HUD_CMD` line and its block of examples. A line that starts with `#` is a comment and is ignored. Setup keeps a block at the end of the file that lists every setting above, commented out with its default, between `# >>> sitrep settings` and `# <<< sitrep settings <<<`. Uncomment a line there or, better, put your own line above the block, since setup replaces the block as a whole each time it runs (and `/sitrep:setup remove` takes it out).

## For Programs

`sitrep ls --json [-a] [-n items | --full]` prints one JSON document that describes the running sessions, and is meant for a program that draws its own view. `-a` includes ended sessions, `-n` caps the items listed in each queue (default 5, or `SITREP_QUEUE_ITEMS`), and `--full` lists them all. It follows these rules:

1. It exits 0 and prints a valid document even with no sessions (`"sessions": []`). On an internal error it exits non-zero, prints a message on stderr and prints nothing on stdout.
2. Every text field is a single line with no control or format characters, so it is safe to draw. A program should still strip them again.
3. `version` is an integer that changes only when a field is removed or its meaning changes. New fields can appear without a new version, so a program should ignore fields it does not know and refuse a version higher than it was written for.
4. The sessions are those active in the last day, newest first by when their state last changed.

The top level holds these fields:

| Field | Meaning |
|---|---|
| `version`, `generated` | The contract version, and the time of the listing in epoch seconds. |
| `registry` | `ok` when liveness came from Claude Code's session registry, `unavailable` when it fell back to the hook states. |
| `settings` | `queue_items`, `highlight_seconds`, `highlight_sgr`, `theme` and `colors`, as set in the environment or `sitrep.conf`. |
| `hidden` | `ended` counts the finished sessions left out, and `stranded_items` counts the queue items they still hold. |
| `inbox` | `count` is the true number of items, and `items` holds the first ones, up to the cap (all of them with `--full`). |
| `sessions` | One object per session, below. |

Each session has these fields:

| Field | Meaning |
|---|---|
| `id`, `short`, `alias` | The full session id, its first four characters, and the name the user gave the session (empty without one). Pass `id` to a command, never `alias`. |
| `running` | False for an ended session, which appears only with `-a`. |
| `state` | `working`, `waiting`, `ended`, `running` (alive in the registry although the hooks say ended), or `-` (no state recorded). |
| `state_label`, `age` | The text the listing shows, such as `working 1m` or `idle? 25m`, and the seconds since the state changed. |
| `state_changed` | When the state changed, in epoch seconds. |
| `quiet_secs`, `idle` | `quiet_secs` is how long the transcript has been quiet, and null unless the session is working. `idle` is true when that is over ten minutes. |
| `needs_input`, `snoozed` | `needs_input` is true for a running session in the `waiting` state: Claude has stopped, and the next move is the user's. It stays true until the user sends a prompt, unless the session is snoozed. `sitrep snooze <id>` mutes it until the session next changes (`snoozed` is then true and `needs_input` false), and `sitrep snooze --off <id>` ends the mute at once. |
| `pid` | The live process of the session, from the registry, or null when it is not known here (another pid space). If a resume left several, the first. A hint only: it can be stale by the time it is used. |
| `pinned`, `highlight` | Whether the Objective is pinned, and whether the session changed within `highlight_seconds`. |
| `dir`, `cwd` | The last part of the working directory, and the whole path. |
| `objective`, `progress`, `now` | The three lines of the status block. |
| `queue` | `count` is the true number of items, and `items` holds the first ones, up to the cap (all of them with `--full`). |

## How It Works

State lives in `~/.claude/sitrep/`: a folder `sessions/<session_id>/` per session, holding `notes`, `queue`, `cwd`, `state`, `count`, `stamp`, `pending`, `pinned`, `snooze` and `alias`, then `inbox.queue` for the inbox, `bin/` and `sitrep.conf`. Version 0.4 and before kept every file in the root; `/sitrep:setup` moves them, and `sitrep migrate [--dry-run]` does the same by hand (it never overwrites, merges queues, and is safe to run again). The program is `~/.claude/sitrep/bin/sitrep`. The status line only reads files, so it renders instantly. The hooks run `sitrep hook`, which answers within milliseconds; the Haiku call runs afterwards in a detached child of the same program, is capped at $0.05 per call, and sets `SITREP_SUMMARIZING` so its own hooks do not recurse. At most one runs per 45 second window. A `Stop` inside the window schedules one deferred run for its end, and a result that arrives after a newer prompt is dropped.

## Privacy And Safety

Session state is stored in `~/.claude/sitrep`, which is owner-only (`700`, files `600`). Prompt and model text is treated as untrusted terminal input, so control characters (escape sequences and the C1 range) and format characters (such as bidi overrides) are removed before it is stored and again before it is printed. The summarizer sends the tail of the transcript to Haiku through `claude -p`, the same service the session itself uses. It runs from an empty temporary directory with no tools, no MCP servers, no settings and a 60 second limit, in its own process group that the limit ends whole, and only reads transcripts that live under `~/.claude/projects`. The daily update check is the only other network use: it asks this repository for its tag names and sends nothing from your sessions.

## Development

`make test` runs `gofmt`, `go mod tidy -diff`, `go vet` and `go test -race`, in CI on Linux and macOS. `make build` builds `./sitrep`. The tests use a throwaway `$HOME` and a stub `claude`, so they never touch real settings or the API. The differential tests compare with the last shell version, the git tag `bash-final`, and need git, jq, perl and that tag. `docs/GO-PORT.md` describes the port from shell to Go.

## Status

The queue does not feed work to Claude automatically. Whether a `Stop` hook should hand over the next item is TBD, as an unattended session that keeps itself busy is also a runaway session. Pushing the same text to the cmux sidebar has not been built; the state is plain files, so a script can read it.

## License

MIT. See `LICENSE`.
