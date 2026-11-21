# Jump backends -- iTerm2 and Terminal.app, and a setting for the rest

Status: proposal, 2026-11-21. Nothing here is built.

## Claim

`enter` in the dashboard jumps to the terminal that shows a session. Today that works in tmux and cmux, and only there. We should add iTerm2 and Terminal.app as built-ins, and add one setting that lets the user turn jumping off, pick a backend, or run a program of their own. We should not add every terminal. A user who needs another one writes a short script and sitrep carries no code for it.

## Today

`internal/tui/jump.go` and `cmux.go`. `jump` reads the environment (`TMUX`, `CMUX_SURFACE_ID`, `CMUX_WORKSPACE_ID`), tries tmux first and then cmux, and says so when neither applies, or when it cannot find a pane. Both backends do the same two jobs: find the pane or surface that holds one of the session's process chains, and focus it. The chains come from the session's process id, and from the `claude attach` client for a session that runs in the background. Every id passed back to a program is one the program gave us, never a name.

## Which backends

- **tmux and cmux** stay built in and detected from the environment.
- **iTerm2 and Terminal.app** are added. They are the two terminals that matter on macOS, and both are driven the same way, from `osascript`.
- **Not added:** GNU screen (little used now), Zellij, kitty, WezTerm, Ghostty, Warp, Alacritty, VS Code's terminal and Windows Terminal. Some have a usable command line and some have none. Each is a backend we could not test here, for few users. The `cmd` setting below covers anyone who wants one.

## iTerm2

iTerm2 can be driven from the command line with `osascript`, and its AppleScript interface lists windows, tabs and sessions with the tty of each. Sitrep already has the tty of a session's process, so the backend needs no walk up the process tree: it asks iTerm2 which session has that tty, then selects its window, tab and session. We have to confirm the exact calls against a real iTerm2 before we build it.

Detection is `TERM_PROGRAM=iTerm.app` or `ITERM_SESSION_ID`.

- **Permission.** The first time, macOS asks whether the program may control iTerm2. If the user says no, the jump fails with a message that says what to allow, and changes nothing, as a jump that finds no pane does today.
- **Order.** Inside tmux inside iTerm2, both variables are set. The jump tries tmux, then cmux, then iTerm2, then Terminal.app, so the pane inside tmux wins and the terminal application is the last resort.
- **Tests.** A fake `osascript` in the tests, as with the fake `tmux`, and one check by the user in a real iTerm2 and a real Terminal.app.

## Terminal.app

macOS's own Terminal is scriptable in the same way, and its AppleScript interface gives each tab a tty. The backend is the iTerm2 one with another application name: ask Terminal which tab has the session's tty, select the tab and bring its window to the front. The exact calls need the same check against the real application.

Detection is `TERM_PROGRAM=Apple_Terminal`. It has the same permission prompt as iTerm2, and the same failure message when the user says no. It is tried last, after iTerm2, though the two cannot both be the terminal in front of one process, so the order only matters if the variables are inherited from a parent.

## The setting

A `jump` key in `sitrep.conf`, with these values:

- `auto` (the default): detect from the environment, in the order above.
- `none`: `enter` does not jump, and says that jumping is off.
- `tmux`, `cmux`, `iterm2` or `terminal`: use only that backend, for someone who runs two and wants a fixed choice.
- `cmd:<program>`: run a program of the user's own.

It is read in the dashboard and not in a hook. `--quit-on-jump` works with every value.

## A program of your own

The program is run with no shell and gets what it needs in its environment, so nothing is parsed from a string:

- `SITREP_JUMP_ID` -- the session's `state.ID`, never its alias.
- `SITREP_JUMP_PIDS` -- the process ids that may hold the session, one per line of the chain (the session's own, and the `claude attach` client's).
- `SITREP_JUMP_TTY` -- the tty of the session's process, when it has one.

It exits 0 when it has jumped. Any other exit code means it could not, and its first line of output becomes the message. That text is cleaned before it is shown (invariant 1).

It is run only when the user presses the key, never from a hook, so it cannot hold up a prompt (invariant 4). It has the same time limit as the built-in jump (3 seconds), runs in its own process group that the limit kills whole (invariant 7), and runs in an empty working directory. It is the user's own program, so we trust what it does and not what it prints.

## Questions

- Whether `jump` can name a list, such as `tmux,iterm2`, to fix both which backends and their order. We lean to a single value at first.
- Whether the dashboard should say which backend it chose, as a line in the key help. A user who sees nothing happen would learn something.
- Whether sessions on another host (see `SHARED-STATE.md`) show the key at all. They cannot be jumped to, so the key should say so and not fail quietly.

## Not proposed

- A backend for every terminal.
- Jumping on its own, such as when a session starts waiting. The bell already says that, and a focus that moves without a key press takes the user's place.
- Running the user's program from a hook, or with any text a model wrote.
