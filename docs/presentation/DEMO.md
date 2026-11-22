# Demo script

Status: draft, 2026-11-21. Nothing is set up or recorded yet. It is the script behind the Demo slide in `sitrep.md`.

## The story

Two real Claude Code sessions work on two made-up projects: a shop's API and its web front end. We open on the payoff, one screen that shows which session needs us, and then show how the work got there: it is caught in the inbox, handed out, finished, and moved when it lands in the wrong place. We end on the same screen, changed. It runs about four minutes once the waiting is cut.

A ninety second version plays scenes 1 to 3 only.

## Everything is real

Both sessions are real Claude Code sessions, so the hooks, the summaries and the jump are the real ones, and nothing is staged. The cost is a few cents, since only the summary call uses a model and it is capped at $0.05 a call.

The demo needs a state folder of its own. Otherwise the dashboard would show your real sessions and your real inbox. We set `SITREP_STATE_DIR=~/demo-state` in every demo terminal, so the sessions, the inbox and the settings of the demo live there, and your real `~/.claude/sitrep` is never touched. Claude Code itself, its login and the installed plugin stay as they are.

## One-time setup

1. Make the demo state folder and set it for every demo terminal, before `claude` starts: `export SITREP_STATE_DIR=~/demo-state`. The hooks, the status line and the summarizer take it from the shell that starts `claude`, and so do your own `sitrep` commands.
2. Check that sitrep 0.9.0 or newer is installed (`sitrep version`), since the setting does not exist before it. Nothing else needs installing: the program and the plugin are the ones you already have.
3. Make the two projects under `~/demo-projects`, each small enough that a task takes one turn:
   - `shop-api`: a few files with a `list_orders` function and a flaky test.
   - `shop-web`: a few files with a settings page and a stylesheet.
4. Set up cmux for the screen: a large font, a theme with good contrast, a short prompt, and a window size that records well. Allow screen recording if cmux asks.
5. Do one dry run end to end, and check that `sitrep ls` in a plain terminal, without the variable, shows your real sessions and none of the demo.

Between takes, empty the demo inbox with `sitrep rm inbox` and run `sitrep prune`, or keep a copy of `~/demo-state` from before the first take and put it back. Starting over completely is deleting that one folder, by its literal path.

## Cue prompts

Real sessions do what the prompt says, so the demo uses fixed prompts that behave the same on every take. Test each one in the dry run and adjust the wording until it does.

- **api, to start working:** "In `app.py`, add `limit` and `offset` to `list_orders`. Then write unittest tests for them in `test_orders.py` next to it and run them with `python3 -m unittest test_orders`. Keep it small. Do not ask, just do it." It is longer than one edit so that `api` is still `working` when the recording starts. `reset` removes the `test_orders.py` it writes.
- **web, to make it stop and ask:** "Before changing anything, ask me in plain text, without using a question tool, whether dark mode should follow the system setting or a toggle. Then stop and wait for my answer." This is what turns the `web` row pink-red, as `needs_input`.

  The cue must end the turn. Sitrep marks a session `waiting` (pink-red) only when the `Stop` hook fires, so a question tool or a permission dialog, which blocks in the middle of a turn, leaves the row at `working`. Type the cues by hand: a paste from a terminal can bring a quote bar (`▎`) into the Objective.
- **api, if it stops to ask:** a stopped `api` shows `waiting`, and the glance needs it `working`. The "Do not ask" in the cue is there for that.

## Scenes

Each scene lists what we do and what the audience should see. The sessions are started and have each finished a turn before the recording starts, so the summaries are already filled in.

### 1. The glance (45 s)

Start the dashboard first, with both sessions idle, then give `api` its cue and `web` its question prompt while it is on screen:

```
sitrep dash
```

The audience sees each row appear and change, since `api` finishes in a few seconds. Both sessions on one screen, each with its state, queue, Objective, Now and how much of its context window is left. `api` goes from working to waiting, and `web` has stopped to ask. Its row turns pink-red and the bell rings. Press `enter`: the terminal jumps to the `web` surface in cmux. This is the whole pitch, shown before any explanation. Say what it replaces: opening each terminal in turn to find out.

### 2. Capture without breaking flow (45 s)

In the `api` session, in the middle of a task:

```
+inbox look at the slow dashboard query later
```

The block under the prompt shows `Inbox: 1`, and the task goes on. Say it: a `+` prompt is never sent to Claude, so a thought has somewhere to go without costing a turn.

### 3. Triage (45 s)

The inbox gets a few more, from a shell:

```
sitrep add inbox 'fix the flaky checkout test'
sitrep add inbox 'add pagination to the orders endpoint'
sitrep add inbox 'check the contrast in dark mode'
sitrep ls
sitrep alias <id> api
sitrep alias <id> web
sitrep move inbox api 2 --item 'fix the flaky checkout test'
sitrep move inbox api 2 --item 'add pagination to the orders endpoint'
sitrep move inbox web 2 --item 'check the contrast in dark mode'
sitrep ls -v
```

The first `ls` shows four-character ids, as the status line does, and the two `alias` commands use them. After that the first column shows names, and an id still works anywhere a name does. Each `move` takes an item from the inbox to the end of a session's queue, and `ls -v` shows every queue under its session. The inbox starts with the slow-query item from scene 2, so the items to move are at position 2, and `--item` makes each move refuse if that position holds anything else. The slow query stays in the inbox.

### 4. Work and finish (40 s, cut for time)

In `api`, take the next item from the queue and give it to Claude as a normal prompt. The state changes to `working`, and the Now line follows what Claude is doing. When the turn is done:

```
sitrep done api
```

`done` prints the item it removed. It is `rm` under a name that reads right: there is no separate "done" state, and removing an item is how it is finished. The dashboard's `dd` does the same. Back in the dashboard, the queue count goes down as we watch. A real turn takes longer than the scene, so it is cut or sped up when the recording is edited.

### 5. Fix a wrong turn (20 s)

The pagination item was for the web side after all:

```
sitrep move api web 1 --item 'add pagination to the orders endpoint'
```

In the dashboard it is `m`, which opens a short list of queues.

### 6. It is just commands (30 s)

```
sitrep add web 'update the settings page copy'
sitrep ls --json | jq '.sessions[] | {id, alias, state, queue: .queue.count}'
```

Everything the dashboard does is a `sitrep` command, so a script can fill a queue. `--json` is a versioned format for tools of your own. The recipe for filling the inbox from GitHub or Linear is in `INBOX-SOURCES.md`.

Then back to `sitrep dash`: the rows now show a different picture from the opening, with the queues moved, one item finished and one session waiting.

## Recording

- **By hand.** Run the demo in a cmux window and record the screen, for example with `cmux record start --format mp4`, or with the system's screen recorder. This is the simplest while we rehearse.
- **Driven by a script.** `demo.sh` types the cues and commands into the three demo terminals with `cmux send`, and waits on sitrep's own state (`sitrep ls --json`), not on the screen. Set `DEMO_API`, `DEMO_WEB` and `DEMO_SHELL` to their cmux surfaces (`cmux list-pane-surfaces`), run `./demo.sh check`, then `reset` (it restores the two projects, removes the tests `api` writes, frees the names `api` and `web` that an earlier take left taken, and empties the inbox), then `scene1` to `scene6` or `all`. `scene1` starts the dashboard and then runs `cue-api` and `cue-web` on screen; `cue` does both when no dashboard is open. The surfaces have no default, so it never types into the terminal it runs in. It types into the shell only at a bare prompt (it stops if a program such as `sitrep dash` is running there or a draft is typed), clears the leftover `+inbox` text from the `api` input box after scene 2, and waits `DEMO_BEAT` seconds (default 3) after each shell command so a viewer can read it. With `DEMO_AUTO=1` it also waits `DEMO_GAP` seconds (default 4) between scenes and at the end, where it would otherwise wait for you. Scene 1 stays by hand, since the jump moves focus. It saves effort only if we record many takes.
- **Editing.** Real turns take ten seconds to a minute. Cut the waiting in scenes 1 and 4, or speed it up and say so on screen.
- **In the deck.** Marp builds one HTML file. A recording is a short video on the Demo slide, or one slide per scene with the commands on it as a fallback when nothing can play.

## Questions

- Whether `sitrep dash` runs full screen or in a popup (`--quit-on-jump`).
- Whether the script should name the other tools. We lean to no: the comparison slide has them.
