# Comparison With Other Tools

This compares claude-sitrep with other tools that help you keep track of several Claude Code sessions. It was written on 2026-11-20 from the public READMEs, documentation and articles listed at the end. Rows for claude-valet and claude-queue were added on 2026-11-22, after a search of GitHub for the topics `task-queue` and `claude-code-plugin`. The author did not run the other tools, so treat their rows as a starting point and check the current state of a tool before you decide. This space changes fast.

## Short Version

sitrep keeps two things per session: a short situation report (Objective, Progress, Now) and a queue of work you plan to do next, with an inbox for work you have not yet matched to a session. Most other tools show what sessions are doing and stop there. Tools that do hold tasks mostly track the agent's own task list, or start and supervise agents, rather than keeping your list. Two small plugins are closer: claude-valet keeps a list of your own parked thoughts, and claude-queue keeps a queue that Claude works through by itself.

Choose sitrep if you want the whiteboard role: a place to write down the next items for each session without leaving the session, shown under the prompt. Choose something else if you need cost tracking, approvals from a dashboard, agent launching, or support for agents other than Claude Code.

## What sitrep Is Built Around

1. **Your list, not the agent's.** The queue belongs to you. It is never fed to Claude automatically, and Claude changes it only when you ask. `+ <text>` queues an item from inside a session, and Claude never sees that prompt.
2. **Plain files and one Go program.** State is files under `~/.claude/sitrep/`. The status line reads files, so it renders instantly. There is no server, no database and no daemon, and no other tool is needed beyond Go to build it.
3. **A summary of each session.** A short Haiku call after each turn writes the Objective, Progress and Now. You can pin the Objective.
4. **Read-only views and a stable data contract.** `sitrep dash` shows every running session in a full-screen dashboard, and `sitrep status` prints one frame. `sitrep ls --json` has a versioned format, so another program can draw its own view.
5. **A state that says who is waiting.** The hooks record `working`, `waiting` and `ended`, and `needs_input` is true while Claude has stopped and the next move is yours.

## Where sitrep Is Weaker

1. **It spends money and sends text out.** Each summary is one `claude -p` Haiku call on the tail of the transcript. A call is capped at $0.05 and at most one runs per session in each 45 second window. That is an upper bound of about 80 calls and $4.00 per session-hour, and only if a turn finishes every 45 seconds all hour. Real use is far lower, but the cost has not been measured. Tools that read local logs only, such as claudectl, send nothing anywhere.
2. **No cost tracking.** sitrep shows no token counts, spend or burn rate. claudectl and OpsDeck do.
3. **Read-only.** You cannot approve a tool call, send input to a session or start one from the dashboard. Several of the tools below can.
4. **Claude Code only.** It depends on Claude Code hooks. Some dashboards also handle Codex, Gemini CLI or opencode.
5. **One machine.** State is local files. A tool with a server can show sessions from several machines.
6. **It changes your settings.** The plugin adds four hooks and `/sitrep:setup` wraps your status line in `~/.claude/settings.json`. Another tool that replaces the status line, such as Claude HUD's setup, hides the sitrep block until you run `/sitrep:setup` again.
7. **The queue does not run itself.** Whether a `Stop` hook should hand over the next item is undecided. claudectl has dependency-ordered task runs.
8. **A small race in the inbox.** An `add` that lands while a `move` is rewriting the same queue can be lost. See The Queue in the README.

## Alternatives

| Tool | Form | What it does | Compared with sitrep |
|---|---|---|---|
| Agent View (Anthropic) | Built into Claude Code, research preview | Lists sessions grouped by state: Working, Needs input, Ready for review, Completed | First party and nothing to install. As described, it shows state across sessions. I found no per-session queue or inbox. |
| Plate-Spinner | Rust crate, dashboard | Status and summaries of each session | The closest to sitrep's report side. I found no queue. |
| claudectl | Rust terminal UI | Live status, cost and burn rate per session, blocked-session detection, desktop and webhook notices, dependency-ordered multi-session task runs, optional local-model approvals | Stronger on cost, alerts and orchestration. It reads local session files and logs and sends nothing out. It runs work for you, where the sitrep queue is a list for you. |
| claude-valet | Claude Code plugin (Python: hooks, an MCP server and a script) | `/valet:park <text>` stores a follow-up before the model sees it (a hook blocks the turn, so it costs no tokens), and `/unpark` brings one back through a native picker as the next piece of work. It has a queue file for each project, and an optional count for the status line. | The closest in idea: the same trick as `+ text`, and the same rule that nothing fires on its own. As described, it keeps its queue for each project, not each session, and I found no inbox, no report of Objective, Progress and Now, and no dashboard. Picking an item hands it to Claude, where sitrep leaves that to you. It is heavier to install: a clone, `install.sh`, Python 3.10 or newer with two packages, and a restart of Claude Code. |
| claude-queue | Claude Code plugin (Python standard library: hooks and a script) | `/queue:add` takes tasks and goals, and Claude works through them in order. A `Stop` hook starts the next item when Claude ends its reply with `QUEUE_DONE`, and a goal is judged by a separate `claude -p` model. It has a JSON queue file for each session. | The opposite design: a queue that runs for Claude, where the sitrep queue is a list for you. It fits when "done" can be checked (tests pass) and you want a run without you. Goal checks use your Claude usage, and pause limits stop a loop. I found no status line or dashboard. |
| OpsDeck | Terminal UI | Finds sessions automatically, shows a daily briefing across projects, cost analytics through ccusage | Stronger on cost and history. |
| claude-code-kanban, claudedash, atc-claude-kanban | Local web boards | Show Claude Code's own task list and session activity as a board. claude-code-kanban lets you add a task by hand in the Pending column | They mirror the agent's tasks. sitrep keeps your own per-session list. They need a local server and a browser tab. |
| Vibe Kanban, Nimbalyst, agent-viewer | Boards, apps or a tmux-based web board | Sessions are cards. Some start, resume and manage agents, and agent-viewer works with agents in tmux | Heavier and agent-centred. Better if you want to launch and supervise agents. |
| baton-os | Filesystem-first kanban | A Scrum-style board kept in files | Also file-based, but a project board, not a status line. |
| tracon | Terminal and browser monitor | Spots stuck, blocked and drifting subagents and worktree file conflicts that would hit a merge | A different problem: parallel subagents on one codebase. |

## Which To Use

1. You want a place to write the next steps for each session, shown under the prompt: **sitrep**.
2. You want one screen of every session's state with nothing to install, and no queue: **Agent View**.
3. You want to park a thought in the middle of a task and pull it back later, and nothing more: **claude-valet**, which works as sitrep's `+ text` does. Choose sitrep if you also want a queue for each session, an inbox, the report under the prompt and a dashboard.
4. You want Claude to work through a list on its own, with goals it checks: **claude-queue**.
5. You care about spend, stalls and approvals, or want to run queued work in order: **claudectl** or **OpsDeck**.
6. You think in boards and want to launch agents from them: **Vibe Kanban** or **Nimbalyst**.
7. You run many subagents against one repository: **tracon**.

They are not exclusive. sitrep keeps its own files and prefixes its status line to whatever you already use, so it can run next to Agent View or a cost monitor.

## Sources

1. NYU Shanghai, [Anthropic Ships Agent View](https://rits.shanghai.nyu.edu/ai/anthropic-ships-agent-view-a-multi-session-dashboard-for-claude-code/)
2. [Plate-Spinner](https://docs.rs/crate/plate-spinner/latest)
3. [claudectl README](https://docs.rs/crate/claudectl/0.20.0/source/README.md)
4. [OpsDeck](https://github.com/getopsdeck/opsdeck)
5. [claude-code-kanban](https://github.com/NikiforovAll/claude-code-kanban), [claudedash](https://yunusemrgrl.github.io/claudedash), [atc-claude-kanban](https://github.com/atc-net/atc-claude-kanban)
6. [Nimbalyst, Best Tools for Managing Parallel AI Coding Agents in 2026](https://nimbalyst.com/blog/best-agent-management-tools-2026/)
7. [agent-viewer](https://blog.brightcoding.dev/2026/09/24/hallucinogenagent-viewer-kanban-board-for-claude-code-in-tmux)
8. [codeongrass, Multi-Agent Monitoring in 2026](https://codeongrass.com/blog/multi-agent-monitoring-agent-quest-baton-os-teamfuse/) (baton-os)
9. [tracon](https://raw.githubusercontent.com/phalladar/tracon/main/README.md)
10. [claude-valet](https://github.com/YeautyYE/claude-valet), README (a dual MIT and Apache 2.0 licence, last pushed 2026-06-03)
11. [claude-queue](https://github.com/Mrcyberwarri0r/claude-queue), README (MIT, last pushed 2026-10-20)
