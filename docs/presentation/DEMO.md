# Demo script

Status: draft, 2026-11-21. Nothing is staged or recorded yet. It is the script behind the Demo slide in `sitrep.md`.

## The story

Three sessions are running for one project: an API, a web front end and the docs. We open on the payoff, one screen that shows which session needs us, and then show how the work got there: it is caught in the inbox, handed out, finished, and moved when it lands in the wrong place. We end on the same screen, changed. It runs about four minutes.

A ninety second version plays scenes 1 to 3 only.

## What is staged, and what is real

The state is plain files, so most of the demo can be staged without a model call. A script writes three sessions into a scratch `HOME`, as the tests do, and the dashboard shows them as running. Two parts cannot be staged honestly:

- **Inside a session** (the `+` prefixes and the block under the prompt) only exists in a real Claude Code session, because a hook reads the prompt. Scene 2 is a short real recording. The `+` prompts never reach the model. Only the summary call spends anything.
- **Jumping with `enter`** needs a real terminal multiplexer to jump in. The staged sessions run as dummy processes in three tmux windows, so the jump has a pane to find. tmux is not installed on this Mac yet.

Everything runs in a scratch `HOME`, never in the real `~/.claude`.

## Scenes

Each scene lists what we do and what the audience should see.

### 1. The glance (45 s)

```
sitrep watch
```

Three sessions on one screen, each with its state, queue, Objective, Now and how much of its context window is left. The staged `docs` session stops and asks a question. Its state becomes `needs_input`, the row turns pink-red and the bell rings. Press `enter`: the terminal jumps to its tmux window. This is the whole pitch, shown before any explanation. Say what it replaces: opening each terminal in turn to find out.

### 2. Capture without breaking flow (45 s, real recording)

In a real Claude Code session, in the middle of a task:

```
+inbox look at the slow dashboard query later
```

The block under the prompt shows `Inbox: 1`, and the task goes on. Say it: the `+` prompt is never sent to Claude, so a thought has somewhere to go without costing a turn.

### 3. Triage (45 s)

The inbox holds a few more by now. From a shell:

```
sitrep add inbox 'fix the flaky checkout test'
sitrep add inbox 'document the new rate limit'
sitrep add inbox 'add pagination to the orders endpoint'
sitrep ls
sitrep alias 15f1 api
sitrep alias 7c20 web
sitrep alias a9e4 docs
sitrep move inbox api 1
sitrep move inbox docs 1
sitrep move inbox web 1
sitrep ls -v
```

The first `ls` shows four-character ids, as the status line does. After `alias`, the first column shows names, and an id still works anywhere a name does. Aliasing takes one line here, because it makes the next commands readable. Each `move` takes an item from the inbox to the end of a session's queue, and `ls -v` shows every queue under its session.

### 4. Work and finish (40 s)

The staged `api` session flips to `working`, and its Now line reads "Fixing the flaky checkout test". When it is done:

```
sitrep rm api
```

`rm` prints the item it removed. There is no separate "done" state. Removing an item is how it is finished, and the dashboard's `dd` does the same. Back in the dashboard, the queue count changes as we watch.

### 5. Fix a wrong turn (20 s)

The pagination item was for the web side after all:

```
sitrep move api web 1
```

In the dashboard it is `m`, which opens a short list of queues.

### 6. It is just commands (30 s)

```
sitrep add web 'check the contrast in dark mode'
sitrep ls --json | jq '.sessions[] | {id, alias, state, queue: .queue.count}'
```

Everything the dashboard does is a `sitrep` command, so a script can fill a queue. `--json` is a versioned format for tools of your own. The recipe for filling the inbox from GitHub or Linear is in `INBOX-SOURCES.md`.

Then back to `sitrep watch`: the three rows now show a different picture from the opening, with the queues moved, one session finished and one waiting.

## Producing it

- **Staging.** One script, kept next to this file, builds the scratch `HOME`, starts three dummy processes in three tmux windows, writes the live-session registry entries and the state files, and flips a session's state on a timer for scenes 1 and 4. It must refuse to run against the real `HOME`.
- **VHS or asciinema.** VHS writes the demo as a `.tape` script of keystrokes and pauses, and renders a GIF, MP4 or WebM. It is repeatable and the script lives in the repo, but it needs `ttyd` and `ffmpeg`. asciinema records a real terminal into a small text `.cast` file that plays in the browser with its own player, and the text can be selected. We lean to VHS for scenes 1 and 3 to 6, since they are deterministic, and a real asciinema recording for scene 2, since that is a live Claude session.
- **In the deck.** Marp builds one HTML file. A recording is a short video or an embedded player on the Demo slide, or one slide per scene with the commands on it as a fallback when nothing can play.

## Questions

- Whether scene 2 is recorded or shown live. A recording cannot go wrong on stage, and a live one is more convincing.
- Whether to show `sitrep watch` in a tmux popup (`--quit-on-jump`) or full screen.
- Whether the script should name the other tools. We lean to no: the comparison slide has them.
- Whether the lack of a "done" state is a gap to fix first. Today the answer is `rm`, which is honest but may read as "delete".
