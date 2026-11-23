# Not built yet

Things that have been thought about and are not built. Nothing here is promised. The first part is the list that used to live in the inbox, and each item has a page that holds the detail. The second part is a set of ideas that a synthetic persona (a model playing a kind of user) generated, and it says so.

## Remote machines

sitrep keeps its state on one machine, and [docs/REMOTE.md](REMOTE.md) gives a recipe for watching another machine over ssh. A configured list of hosts could feed extra rows into `sitrep dash` through the same snapshot as `ls --json` (see "If this is not enough" in that page). The shared-folder steps 2 and 3 were dropped, and [docs/SHARED-STATE.md](SHARED-STATE.md) says why.

## A jump setting

`enter` jumps to the terminal of the selected session for tmux, cmux, iTerm2 and Terminal.app. For any other terminal there is no way to turn jumping off, or to run a program of your own. The plan is only the escape hatch: a `jump` key in `sitrep.conf` with the values `auto` (the default), `none`, a named backend, or `cmd:<program>`. The design, and how the program is run, are in [docs/JUMP-BACKENDS.md](JUMP-BACKENDS.md).

## A check for a replaced status line

Another tool's setup, such as Claude HUD's, can replace the status line, and the sitrep block then disappears without a word until `/sitrep:setup` runs again. A `SessionStart` check could notice that the status line is no longer ours (it ends in the tag `# claude-sitrep`, see invariant 6 in `AGENTS.md`) and say so once. It was left out of the first version, because a hook must never fail, hang or swallow a prompt (invariant 4), and the check must cost nothing when the status line is fine.

## Documented elsewhere

- Whether a `Stop` hook should hand over the next queued item on its own is undecided: [docs/QUEUE-NEXT.md](QUEUE-NEXT.md), option 2.
- Pushing the queue to the cmux sidebar is not built: [docs/HOW-IT-WORKS.md](HOW-IT-WORKS.md), "Not built".

## Candidates from the power-user persona, checked

These ideas were not asked for by users. A synthetic persona ([docs/personas/power-user.md](personas/power-user.md)) generated ten of them in a run against sitrep 1.0.1 on 2026-11-22: a model playing a developer who walks through a day, with one session and with about eight, and reports the pain it finds. It worked from the documents and from `sitrep ls` on sample sessions. Its first list was then checked, idea by idea, against what sitrep already does (the code, `sitrep help`, the JSON fields and the docs) and against what Claude Code documents (a reading of its documentation on 2026-11-22; the list of notification types came from a summary of a long page, not a verbatim read). The pain each one names has not been confirmed with people who use the tool.

| # | The persona's idea | What exists today | Verdict |
| --- | --- | --- | --- |
| 1 | A recap after a break | The Objective, Progress and Now are rewritten after every turn. Claude Code has its own one-line recap on return and `/recap`. Nothing in sitrep reads the repository. | The narrative is covered twice. The repository half is real: see A. |
| 2 | Notifications when away | Only the dashboard's bell. Claude Code's `Notification` and `Stop` hooks can already run any command. | Not sitrep's to build: Claude Code does it. See "What the persona would not build", and C. |
| 3 | A git and pull request snapshot | Nothing: the JSON has no branch or repository field. | Real: see A. |
| 4 | A daily and weekly digest | `sitrep log --since 1d --op queue.rm` lists the items done. | Mostly exists: see B. |
| 5 | Search across sessions | `sitrep log --json \| jq` searches queue text, and `grep -r` over `sessions/*/notes` finds the latest summary of a session still on disk. | Mostly exists: see B. |
| 6 | Relations between sessions | Nothing. | Dropped: an item's text can say "after 2f3a", and there is little evidence of the pain. |
| 7 | A cost ledger | Nothing is recorded, but the status-line input that sitrep already reads carries the session's cost. | Not worth building: claudectl and OpsDeck do it better, and sitrep is meant to run beside them. See "What the persona would not build". |
| 8 | An event stream | There is no `watch` command; scripts poll `sitrep ls --json`. | Not worth building yet: a hook layer on top of Claude Code's hooks, and a polling loop does it. See "What the persona would not build". |
| 9 | A multi-host view | The ssh loop and a note in [docs/REMOTE.md](REMOTE.md). | Duplicate of "Remote machines" above. |
| 10 | A command from a queue item | `Y` in the dashboard copies a session's `claude --resume <id>`. | Dropped: starting a session from an item adds little, and running it would break the read-only dashboard. |

### A. The repository state of each session

Pain, as the persona put it: in the morning triage the user changes into each working directory to learn the branch, whether the tree is dirty and whether CI passed, and after a break runs `git status` and `git diff --stat` by hand. The JSON of `sitrep ls` has `cwd` and nothing about the repository.

Idea: branch, dirty flag and ahead or behind in `sitrep ls --json` and as a dashboard column, from local `git` reads that the hook saves in the session folder, with a bounded wait (invariant 4) and cleaned text (invariant 1). Pull request and CI state through `gh` only as an opt-in, in a detached refresh like the update check, which would be new network use (invariant 10). The status-line input has a `worktree` object and `cost.total_lines_added` and `total_lines_removed`, which cost nothing to record and say how much a session has changed; what is inside `worktree` was not checked. Claude Code already recaps what a session did, so sitrep would add the repository facts, which are exact after a compaction or after edits made outside the session, and which can be read for sessions the user is not in.

Doubt: for one session the user is in, asking Claude "what changed?" works, and Claude can run `git` itself. The value is in the overview of many sessions. No tool in [docs/COMPARISON.md](COMPARISON.md) does it.

### B. A daily view built on `sitrep log`

Pain: "what did I finish yesterday", for a stand-up, is answered from memory.

What exists: with `SITREP_LOG=on`, [the history log](LOG.md) records each queue change with its text, pins, snoozes, aliases, prunes, and the start and end of sessions. `sitrep log --since 1d --op queue.rm` lists the items done.

What is missing: the log is off by default, because it keeps the text of the user's items. It does not record the Progress and Now summaries, which `notes` overwrites after every turn, and it records an Objective only when it is pinned. It is kept for 10 MB or 30 days. There is no view grouped by day or by session. Search is covered by `sitrep log --json \| jq` for queue text, and what a session did last week can be found only in Claude's own transcripts.

Idea: a `sitrep log --digest` view that groups the items done and the sessions that ended for a day, and perhaps a record of the Objective when a session ends. Turning the log on by default is a decision about privacy, not a feature.

### What the persona would not build

- Notifications through Claude Code's own hooks (the persona's idea 2). Claude Code's documentation lists a `Notification` hook with the types `permission_prompt`, `idle_prompt` and `agent_needs_input`, and a `Stop` hook for the end of a response. Either can run any command, such as a push to a phone, and a hook can return a `terminalSequence` that emits an OSC 9, 99 or 777 notification or a bell. A user can set this up without sitrep, so a recipe for it is a Claude Code tip, not part of sitrep. What sitrep adds is its own meaning of a session that needs you (snooze, fade, aliases), which matters little for a notification at the moment a session stops.
- An event stream, `sitrep watch --json` (the persona's idea 8), or a `SITREP_NOTIFY_CMD` setting. Either would be a hook layer on top of the hooks that sitrep and Claude Code already have, and a script that wants the state can poll `sitrep ls --json`, which is cheap (0.22 s for 3,000 sessions in a robustness test). The pain the persona named was polling in a loop, and polling works. It should wait for someone who has a need that polling does not meet.
- A cost ledger (the persona's idea 7). The status-line input does carry `cost.total_cost_usd` (a client-side estimate at list price, not a bill), the duration, the lines added and removed, and `rate_limits`, and `sitrep statusline` already reads that input, so recording it would be cheap. Cheap is not a reason: claudectl and OpsDeck track cost and burn rate far better, [docs/COMPARISON.md](COMPARISON.md) sends people who need it to them, and sitrep is meant to run beside such a tool. The lines added and removed are still a free signal of how much a session has changed, and idea A may use them.
- Approvals or input from the dashboard: it breaks the read-only rule, and claudectl does it.
- A `Stop` hook that feeds the next item on its own ([docs/QUEUE-NEXT.md](QUEUE-NEXT.md), option 2): an unattended session that keeps itself busy is a runaway session.
- A web interface, a server, or a synced state folder (already dropped).
- Other agents, such as Codex or Gemini: a different product.
- A kanban view of the agent's own tasks: the user's own list is the point.
