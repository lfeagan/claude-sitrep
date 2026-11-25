# Not built yet

Things that have been thought about and are not built. Each has a page that holds the detail, and this page says where. Nothing here is promised: it is the list that used to live in the inbox.

## Remote machines

sitrep keeps its state on one machine, and [docs/REMOTE.md](REMOTE.md) gives a recipe for watching another machine over ssh. A configured list of hosts could feed extra rows into `sitrep dash` through the same snapshot as `ls --json` (see "If this is not enough" in that page). The shared-folder steps 2 and 3 were dropped, and [docs/SHARED-STATE.md](SHARED-STATE.md) says why.

## A jump setting

`enter` jumps to the terminal of the selected session for tmux, cmux, iTerm2 and Terminal.app. For any other terminal there is no way to turn jumping off, or to run a program of your own. The plan is only the escape hatch: a `jump` key in `sitrep.conf` with the values `auto` (the default), `none`, a named backend, or `cmd:<program>`. The design, and how the program is run, are in [docs/JUMP-BACKENDS.md](JUMP-BACKENDS.md).

## A check for a replaced status line

Another tool's setup, such as Claude HUD's, can replace the status line, and the sitrep block then disappears without a word until `/sitrep:setup` runs again. A `SessionStart` check could notice that the status line is no longer ours (it ends in the tag `# claude-sitrep`, see invariant 6 in `AGENTS.md`) and say so once. It was left out of the first version, because a hook must never fail, hang or swallow a prompt (invariant 4), and the check must cost nothing when the status line is fine.

## A choice of card marks, and a style tool

The Cards mark a session by its state with a heavy bracket down the card (`┏`, `┃`, `┗`), and the selection by an arrow and a rail that stops half a line short of it. The half-bar form was tried first: `▖` on the first line and `▌` down the rest, with the rail the full length. Both forms look right; the bracket was chosen. The glyphs are one table in `internal/tui/views.go` (`cardMarks`), so a second form would be a second table.

Offering both is a setting (`SITREP_CARD_MARK=bar|bracket`) that has to travel to the dashboard in the snapshot's `settings`, an additive change to the `ls --json` format. Setting it from `sitrep colors` would take a third known key in invariant 9 of `AGENTS.md` (the tool writes two lines of `sitrep.conf` today), and a row in the tool that previews the card. The tool would then set more than colours, and could be renamed `style`, with `colors` kept as an alias. Not planned: one form is simpler to keep, document and draw in the README pictures.

## Documented elsewhere

- Whether a `Stop` hook should hand over the next queued item on its own is undecided: [docs/QUEUE-NEXT.md](QUEUE-NEXT.md), option 2.
- Pushing the queue to the cmux sidebar is not built: [docs/HOW-IT-WORKS.md](HOW-IT-WORKS.md), "Not built".
