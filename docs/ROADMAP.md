# Not built yet

Things that have been thought about and are not built. Each has a page that holds the detail, and this page says where. Nothing here is promised: it is the list that used to live in the inbox.

## Remote machines

sitrep keeps its state on one machine, and [docs/REMOTE.md](REMOTE.md) gives a recipe for watching another machine over ssh. A configured list of hosts could feed extra rows into `sitrep dash` through the same snapshot as `ls --json` (see "If this is not enough" in that page). The shared-folder steps 2 and 3 were dropped, and [docs/SHARED-STATE.md](SHARED-STATE.md) says why.

## A jump setting

`enter` jumps to the terminal of the selected session for tmux, cmux, iTerm2 and Terminal.app. For any other terminal there is no way to turn jumping off, or to run a program of your own. The plan is only the escape hatch: a `jump` key in `sitrep.conf` with the values `auto` (the default), `none`, a named backend, or `cmd:<program>`. The design, and how the program is run, are in [docs/JUMP-BACKENDS.md](JUMP-BACKENDS.md).

## A check for a replaced status line

Another tool's setup, such as Claude HUD's, can replace the status line, and the sitrep block then disappears without a word until `/sitrep:setup` runs again. A `SessionStart` check could notice that the status line is no longer ours (it ends in the tag `# claude-sitrep`, see invariant 6 in `AGENTS.md`) and say so once. It was left out of the first version, because a hook must never fail, hang or swallow a prompt (invariant 4), and the check must cost nothing when the status line is fine.

## Documented elsewhere

- Whether a `Stop` hook should hand over the next queued item on its own is undecided: [docs/QUEUE-NEXT.md](QUEUE-NEXT.md), option 2.
- Pushing the queue to the cmux sidebar is not built: [docs/HOW-IT-WORKS.md](HOW-IT-WORKS.md), "Not built".
