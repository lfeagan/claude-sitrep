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

## Candidates from the power-user persona

These ten ideas were not asked for by users. They come from a run of the persona in [docs/personas/power-user.md](personas/power-user.md) against sitrep 1.0.1 on 2026-11-22: a model playing a developer who walks through a day, with one session and with about eight, and reports the pain it finds. It worked from the documents and from `sitrep ls` on sample sessions, and it did not drive the dashboard. Treat each as an unchecked idea: what it says about today's behavior (for example what `sitrep log` records, or what is in `ls --json`) has not been verified, and the pain it names has not been confirmed with people who use the tool. They are ranked by the persona, pain first. Each says how it would fit the design (plain files, no server, a read-only dashboard, a queue that is the user's own list), and whether another tool already does it.

### 1. A recap after a break, with the changes in the repository

Pain: after lunch, a meeting or a night, the user scrolls the transcript to recall what was done, then runs `git status`, `git diff --stat` and `gh pr checks` by hand. The summary says what Claude thought it did and nothing about the repository. The persona put it at three to ten minutes, several times a day, for one session or many.

Idea: `sitrep recap [id]`, also shown at `SessionStart`: what the session did, the branch, the files changed since its last turn, and what is left in its queue. Fit: branch and diff stat are local `git` reads that the hook saves in the session folder, so no network, with a bounded wait (invariant 4) and cleaned text (invariant 1). Others: no tool in [docs/COMPARISON.md](COMPARISON.md) does the repository half.

### 2. Notifications when away

Pain: the bell rings only in an open dashboard that someone is looking at, so a session can wait an hour, and a user with one session runs no dashboard at all.

Idea: a setting such as `SITREP_NOTIFY_CMD=<program>`. The hook runs the program once when a session starts needing you, in its own process group with a deadline, and passes the id, the alias and the Now line as environment variables, never on the command line. Dedup is kept in a file. Fit: the user's program does any network use, so sitrep itself stays offline (invariant 10). Others: claudectl has desktop and webhook notices, so this would catch up.

### 3. A git and pull request snapshot for each session

Pain: in the morning triage the user changes into each working directory to learn the branch, whether the tree is dirty, and whether CI passed.

Idea: branch, dirty flag and ahead or behind in `ls --json` and as a dashboard column, from cheap local reads, cached. Pull request and CI state through `gh` only as an opt-in, in a detached refresh like the update check, which would be new network use. Others: none of the compared tools shows it; tracon covers worktree conflicts only.

### 4. A daily and weekly digest

Pain: "what did I finish yesterday", for a stand-up or a status report, is answered from memory.

Idea: `sitrep recap --since 1d`: sessions ended, their last Objective, and the queue items done. Fit: it needs a durable record of ended sessions, so either the history log on by default or a small journal, which is new state and a question of how long to keep it. Others: OpsDeck has a daily briefing.

### 5. Search across sessions

Pain: finding which session did a change last week is a `grep` in the state folder, which the docs say not to do.

Idea: `sitrep find <text>` over Objectives, Progress, queue and log text, ended sessions included, read through `state.ReadFile` with no index. It needs ended sessions to be kept.

### 6. Relations between sessions

Pain: "this waits on that session's migration" lives in the user's head.

Idea: an item tagged `after:<id>`, shown as "waiting on 2f3a". It is display only and nothing starts, so the queue rule holds. claudectl runs dependency-ordered tasks, which sitrep should not copy.

### 7. A cost and token ledger

Pain: the user opens ccusage or claudectl to see spend. A surprise bill is rare, and a worry for people on API billing.

Idea: spend and tokens per session and per day, and a soft limit that feeds notifications. It is worth doing only if the data comes free from the status-line input. [docs/COMPARISON.md](COMPARISON.md) already lists the lack of it as a weakness, and claudectl and OpsDeck do it better.

### 8. An event stream

Pain: people who script around sitrep poll `sitrep ls --json` in a loop.

Idea: `sitrep watch --json`, one line per state change, from a plain loop over the snapshot with no server. It would be the base for notifications (2) and for other tools.

### 9. A multi-host view

Pain: a laptop plus a server means a loop over ssh, which is the recipe in [docs/REMOTE.md](REMOTE.md). It is already under "Remote machines" above; the persona ranked it above the jump setting. It breaks the no-network rule on purpose, so it stays behind the recipe until the recipe is not enough.

### 10. A ready command from a queue item

Pain: copying an inbox item into a fresh terminal.

Idea: print a `claude` command, with its working directory, for an item. Printing is fine; running it would break the read-only dashboard, and Vibe Kanban and agent-viewer do that better.

### What the persona would not build

- Approvals or input from the dashboard: it breaks the read-only rule, and claudectl does it.
- A `Stop` hook that feeds the next item on its own ([docs/QUEUE-NEXT.md](QUEUE-NEXT.md), option 2): an unattended session that keeps itself busy is a runaway session.
- A web interface, a server, or a synced state folder (already dropped).
- Other agents, such as Codex or Gemini: a different product.
- A kanban view of the agent's own tasks: the user's own list is the point.

The persona's closing view: the features that would make it rely on sitrep are the recap with repository changes (1) and notifications (2), and what would turn sitrep into something it should not be is starting, resuming or driving sessions, or queue items that run themselves.
