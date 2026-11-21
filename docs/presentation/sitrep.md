---
marp: true
title: sitrep
description: A situation report and a work queue for every Claude Code session
paginate: true
footer: sitrep -- 2026-11-21
style: |
  section { font-family: -apple-system, "Segoe UI", Helvetica, Arial, sans-serif; font-size: 28px; }
  section h1 { font-size: 1.5em; }
  section.title { text-align: center; justify-content: center; }
  section.title h1 { font-size: 2em; }
  code { font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace; }
  pre { font-size: 0.72em; }
  table { font-size: 0.7em; }
  footer { font-size: 0.5em; }
---
<!--
Build a single self-contained web page (arrow keys to move, p for the presenter view):
  npx @marp-team/marp-cli@4 docs/presentation/sitrep.md -o sitrep.html
Also exports to PDF and PPTX with --pdf and --pptx (these need Chrome installed).
-->

<!-- _class: title -->
<!-- _paginate: false -->
<!-- _footer: "" -->

# sitrep

A situation report and a work queue for every Claude Code session

Lance Feagan
2026-11-21

---

# Context

Running several Claude Code sessions at once is productive, and it is hard to keep in your head. Each one lives in its own terminal.

After an hour away from a terminal, three questions come up again and again:

- What was this session for?
- How far along is it, and what is it doing right now?
- Is it waiting on me, and what should it do next?

The answers are all in the transcript. Nobody wants to scroll a transcript to find out.

<!-- Open with the honest version of the problem: this is not a crisis, it is a tax paid many times a day. -->

---

# Goals

An ideal tool would:

- **Show** each session's purpose, progress and current step, under the prompt where you already look.
- **Hold** what you plan to do next, per session, so a thought has somewhere to go without interrupting the work.
- **Stay local** -- no server, no account, no daemon, and a status line that renders instantly.

---

# What you see

```
Objective: Build a three-line status block for my sessions
Progress:  Wrote the wrapper and hooks; summarizer tested against a fake transcript.
Now:       Waiting on a repo name.
Queue [id a1b2]: 2 items, next: update README
```

The Objective starts as your first prompt, so the block is useful from the first second. A short title replaces it after the first turn, and `sitrep pin` fixes it for good.

<!-- The id a1b2 is how every sitrep command addresses this session. -->

---

# Where the summary comes from

After each turn, a short Haiku call reads the tail of the transcript and rewrites Progress and Now.

- **Capped:** at most $0.05 for a call, and at most one call per session in each 45 second window.
- **Isolated:** it runs with no tools, no MCP and no settings, in an empty directory, under a time limit.
- **Never stale:** a summary that arrives after a newer prompt is dropped.

This is the only part of sitrep that spends money or sends text out, and it can be switched off.

---

# The queue is yours

The queue is a list for you, not instructions for Claude. It is never fed to a session automatically.

- `+ fix the flaky test` adds an item from inside a session, and Claude never sees that prompt.
- `+inbox` queues work you have not matched to a session yet, and `+take` moves an inbox item into this one.
- `sitrep add`, `edit`, `top`, `reorder` and `move` change a queue from any shell.

An unattended session that keeps itself busy is also a runaway session, so we left the handing-over of work to you.

---

# Who is waiting on whom

Hooks record whether a session is `working`, `waiting` or `ended`.

A running session that has stopped and is waiting is `needs_input`: the next move is yours. The dashboard marks it with a pink-red `●`, rings a bell when it newly turns true, and lets you snooze it until the state changes.

This is the answer to "do I need to go back to that terminal?" without opening it.

---

# The dashboard

`sitrep watch` is a full-screen view of every running session, with its state, queue, Objective, Now and how much of the context window is left.

- `enter` jumps to the terminal that shows the session, in tmux or cmux.
- `d` opens a session in full, `i` opens the inbox, `e` opens a session's queue.
- Every key that changes something runs a fixed `sitrep` command after a preview or a confirm, and `--read-only` turns them off.

`sitrep status` prints one frame as text, and `sitrep ls --json` is a versioned format for your own tools.

---

# Plain files, one program

State is files under `~/.claude/sitrep/`: a folder per session, an inbox and one config file. The status line reads files, so it renders instantly.

- One Go program, no database and no daemon. A hook answers in milliseconds, and the summarizer runs afterwards in a detached child.
- Locks are bounded, so a hook can never hang a prompt, and every error ends in exit 0 so it can never swallow one.
- Because it is files, a script, `grep` or a backup tool is already a client.

---

# Demo

<!-- Fill this in when the demo exists. The outline: -->

1. Three sessions, three terminals, each with its status block.
2. `+ <text>` in one of them, then `sitrep watch` to see the queue appear.
3. A session stops and asks a question: the row turns pink-red and the bell rings.
4. `enter` jumps to it.
5. `+inbox`, then `+take` from another session.

---

# What we don't do

- **No cost tracking.** The dashboard shows no tokens, spend or burn rate. Other tools do that well, and sitrep adds to them.
- **No approvals or input.** It is read-only, so it cannot answer a session's prompt for you.
- **Claude Code only.** It depends on Claude Code's hooks, so other agents are out of reach.
- **One machine.** State is local files, so sessions on other machines are invisible (see Next).

---

# Compared with the others

| Tool | Shows | Holds your next steps |
|---|---|---|
| Agent View (Anthropic) | State across sessions, built in | No |
| claudectl, OpsDeck | Cost, stalls, alerts, history | No (claudectl runs queued work for you) |
| Kanban boards | The agent's own task list | Some, by hand, on a local server |
| **sitrep** | Objective, Progress, Now, state | **Yes, per session, plus an inbox** |

They are not exclusive. sitrep prefixes your existing status line, so it can run beside a cost monitor or Agent View.

<!-- Full comparison, with sources and the date it was written, is docs/COMPARISON.md. Say plainly that I did not run the other tools. -->

---

# Safety

Text from prompts, transcripts and models is untrusted terminal input.

- Control and format characters are stripped before anything is stored and before anything is printed.
- State files are owner-only, and a session id can only be letters, digits, `_` and `-`, because it becomes part of a file name.
- `sitrep setup` changes nothing until the settings edit has been checked, backs the file up first, and touches only the keys it owns.

---

# Install

```
/plugin install sitrep --marketplace lfeagan/claude-sitrep
/sitrep:setup
```

The plugin brings the source, the hooks and a skill. A plugin cannot build a program or declare a status line, so `/sitrep:setup` builds the program with Go 1.26 and wraps your status line, after Claude Code asks about the settings change.

Without the plugin: `go install` the `cmd/sitrep` package and run `sitrep setup`.

---

# Next

Four proposals are written down and none is built:

- **Shared state** -- put the state folder on a synced drive, so one machine can watch another's sessions. Each file keeps one writer.
- **History log** -- an opt-in, size-limited record of every change, for "what did I queue last week".
- **Jump backends** -- iTerm2 and Terminal.app beside tmux and cmux, and a `cmd:` setting for any other terminal.
- **Inbox from GitHub and Linear** -- a documented recipe, not code.

---

<!-- _class: title -->

# Fin! Q+A

github.com/lfeagan/claude-sitrep

<!--
Sources:
1. README.md, the design and every number on these slides.
2. docs/COMPARISON.md, written 2026-11-20 from the other tools' public documentation.
3. docs/SHARED-STATE.md, docs/HISTORY-LOG.md, docs/JUMP-BACKENDS.md, docs/INBOX-SOURCES.md, the proposals.
-->
