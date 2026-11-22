# Personas for evaluating sitrep

A persona is a short brief for a subagent that plays one kind of user against sitrep and reports what went wrong for that user. It finds rough edges, unclear text and broken promises. It is Claude's guess at what that user would do, so it does not prove that real users would behave the same way.

## Files

| File | What it is |
| --- | --- |
| `first-timer.md` | Never used sitrep, reads the README and `sitrep help`, and has ten minutes to decide whether it helps. |
| `keyboard-only.md` | Lives in vi and tmux, uses no mouse, and tests what `docs/KEYS.md` and `?` promise against what the keys do. |
| `heavy-user.md` | Twenty sessions in a 70 by 24 pane, no patience for scrolling, and a need to see at once what is waiting. |
| `hostile-input.md` | A security reviewer who tests the claim that text from files, transcripts and models never reaches the terminal as it is. |
| `fixture.sh` | Builds a throwaway state folder (four sessions, queues, an inbox) by writing the files of `docs/STATE-LAYOUT.md`. `--sessions N` adds more of every state and age, and `--hostile` adds files with escape sequences, bidi overrides, huge fields, links and junk. |
| `drive.sh` | Runs the dashboard on the fixture in a detached tmux session, presses keys and reads the screen (`start --sessions N --hostile --size WxH`; `raw` and `title` show the colour codes and the window title). It uses cmux instead when there is no tmux. |

## Run one

1. Install the build you want to test (`make install`), as the run uses the `sitrep` on `PATH`. `SITREP_BIN` names another one.
2. Start a subagent and give it the persona file and these lines:

   > You are the persona in `docs/personas/<file>.md`. Follow its rules. Use only `docs/personas/drive.sh` and the `sitrep` command with the exports from `drive.sh env`. Hand back the report in the format the persona asks for.

   The persona runs `drive.sh start` first and `drive.sh stop` last, even when something fails.
3. Read the report. A finding is a lead: reproduce it before you act on it.

A run starts a tmux server of its own (socket `sitrep-persona-NAME`, 110 by 34 unless `--size` says otherwise), and `DRIVE_NAME` gives each run a name so that several can go at once, so your tmux is not touched, and ends it when it stops. It needs `tmux` or `cmux`, and `bash`. `DRIVE_BACKEND=cmux` forces cmux, which opens a workspace named `sitrep-persona` without taking focus.

## Rules every persona follows

- Only the fixture. Never read or change anything under `~/.claude`, and never run `sitrep setup`, `migrate`, `prune -y` or `log --clear`.
- No source reading unless the persona's brief allows it. A user does not read Go.
- A step budget, so a confused persona stops and reports instead of looping.
- Evidence with every finding: the keys pressed and the screen text seen.
- The fixture has no live Claude sessions, so `enter` (jump) answers that it does not know the session's process. That is expected and is not a finding.

## Report format

Each persona ends with a list of findings, each with: a one-line title, severity (blocked me, confused me, small annoyance, wish), the keys or commands, the screen text it saw, what it expected, and a suggested fix in one sentence. Then three lines: what worked well, the one change that would help most, and whether it would keep using sitrep.

## Adding a persona

Copy a file, change who they are, their goal, their tolerance and what they may read, and keep the rules and the report format. The next ones worth writing are a no-colour or colour-blind user (the capture shows the marks, not the colours), and a remote user over ssh.
