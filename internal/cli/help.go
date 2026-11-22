// The texts below were taken from the bash help in bin/sitrep (tag bash-final) and are the contract.

package cli

// help holds the details of each command, shown by `sitrep help <command>` and `sitrep <command> --help`.
var help = map[string]string{
	"add": `usage: sitrep add <id> <text>

Append an item to a queue. The text is one line of at most 200 characters, and longer
text is cut. Quote text that holds shell characters such as ( ) ; or &.

  sitrep add 15f1 'fix the flaky test (CI)'
  sitrep add inbox 'look at the nightly job'
`,
	"alias": `usage: sitrep alias [<id> <name> | <id> --off]

Give a session a name, to use wherever an id goes (sitrep ls sitrep). A name is 1 to 32
lowercase letters, digits and hyphens, starts with a letter, and is not 'inbox' or the
start of a session id. One session has one name, and a name taken by another session is
refused, so free it first with --off. With no arguments, list the names. An id always
wins over a name, and a name must match whole: 'sit' does not find 'sitrep'.

  sitrep alias c5d0 sitrep
  sitrep alias c5d0 --off
`,
	"edit": `usage: sitrep edit <id> <n> <text> [--item <old text>]

Replace the text of item n, which keeps its place. The text is one line of at most 200
characters, and longer text is cut. It refuses when the queue changed after it was read.
With --item, it also refuses unless item n is exactly that old text, for a program that
showed a list.

  sitrep edit c5d0 2 'add human friendly aliases for session names'
`,
	"rm": `usage: sitrep rm <id> [n] [--item <text>]

Remove item n (default 1) from a queue and print it. It refuses when the queue
changed after it was read, so it never removes a different item. With --item, it
also refuses unless item n is exactly that text, for a program that showed a list.
`,
	"done": `usage: sitrep done <id> [n] [--item <text>]

The same as rm, for an item that is finished: it removes item n (default 1) from a queue
and prints it. It refuses when the queue changed after it was read. With --item, it also
refuses unless item n is exactly that text. There is no separate done state.
`,
	"top": `usage: sitrep top <id> <n> [--item <text>]

Move item n to the front of a queue, so that it is the next item. With --item, it
refuses unless item n is exactly that text.
`,
	"reorder": `usage: sitrep reorder <id> <from> <to> [--item <text>]

Move the item at position from to position to. The other items keep their order. With
--item, it refuses unless the item at from is exactly that text.
`,
	"move": `usage: sitrep move <from> <to> [n] [--item <text>]

Hand item n (default 1) from one queue to another. Either side may be inbox. The item
is added to the destination before it leaves the source, so a failure can leave a
duplicate but never lose it. With --item, it refuses unless item n is exactly that
text, so a program that showed a list never moves a different item.
`,
	"pin": `usage: sitrep pin <id> <text>

Set a session's Objective and stop the summarizer from changing it. Inside a session,
a prompt that starts with "+pin " does the same. See unpin.
`,
	"unpin": `usage: sitrep unpin <id>

Let the summarizer rewrite a session's Objective again.
`,
	"ls": `usage: sitrep ls [-a] [-v] [-n items] [id]
       sitrep ls --json [-a] [-n items | --full]

List running sessions with their state, queue, Objective and Now. With an id, list
that queue instead; an id of inbox lists the inbox.

  -a, --all   include ended sessions
  -v          add the Progress line, and list each queue
  -n items    how many items each queue lists; implies -v, and 0 gives the count and
              the next item only
  --json      print one JSON document for programs, see the README; -n caps the items
              in each queue, and --full lists them all

The default for -n is 5, or SITREP_QUEUE_ITEMS from the environment or sitrep.conf.
`,
	"snooze": `usage: sitrep snooze [--off] <id>

Say that a session which is waiting for you can wait: the dashboard stops drawing it as needing you, and shows a ○ in place of the ●. This is the command behind the z key. The mute ends by itself when the session changes: the next
prompt, or the end of the next turn (for a session at a permission dialog, until the dialog is over). A session that does not need you has nothing to snooze, and the command says so. ` + "`" + `--off` + "`" + ` (before or after the id) ends it at once. Only the JSON changes: the
` + "`" + `needs_input` + "`" + ` field turns false and ` + "`" + `snoozed` + "`" + ` turns true.
`,
	"log": `usage: sitrep log [--session <id>] [--op <op>] [--host <name>] [--since <time>] [-n <count>] [--json] [--clear]

Show the history log: a line for each change sitrep made to a queue, an Objective, an
alias, a snooze or a session, newest last. It is off until you set SITREP_LOG=on in
sitrep.conf (or in the environment). It keeps the text of items, in the state folder,
in log/, and never sends it anywhere. Each entry names its host, and a machine's log is
read on that machine (over ssh for a remote one).

  --session   only this session or queue (id, prefix, alias or inbox); a move shows for both
  --op        only this operation: queue.add, queue.rm, queue.edit, queue.move,
              queue.reorder, pin, unpin, snooze, unsnooze, alias, prune, session.start,
              session.end
  --host      only entries of this host
  --since     only entries newer than this: 90m, 36h, 2d, or a date such as 2026-11-21
  -n          show the latest this many (default 50; 0 shows all)
  --json      one JSON object per line
  --clear     delete the log files, and nothing else

With --json, each line is an entry with the fields e, t, host, id, op and, when they apply, text,
old, from, to and dest. Process it with jq, not the raw files, which can hold repeats and a torn
last line:

  sitrep log --json -n 0 | jq -r 'select(.op=="queue.rm") | .text'

The log is kept to SITREP_LOG_MAX megabytes (default 10) and SITREP_LOG_DAYS days
(default 30), whichever is reached first, so it shows recent history, not all of it.
`,
	"prune": `usage: sitrep prune [-y | -k | --dry-run]

Mark each session that is no longer running as ended. For a session that still holds
queue items, ask whether to move them to the inbox.

  -y          answer yes to every question
  -k, --keep  mark the sessions ended, leave every queue where it is, and ask nothing;
              for a program with no one to answer
  --dry-run   say what would happen, and change nothing

The three options exclude one another.

It reads Claude Code's live-session registry and changes nothing if it cannot. It never
touches a running session or one under two minutes old, and never deletes a queue.
`,
	"dash": `usage: sitrep dash [--tab cards|table|wall] [--interval 3s] [--once] [--width n] [--quit-on-jump]
                    [--read-only] [--no-bell]

A live full-screen dashboard of the sessions, with a Cards tab, a Table tab and a Wall tab (the
sessions as tiles, for many at once) and a status bar.
It reads the sessions itself and changes nothing except through the same fixed commands this
program has (prune, snooze and the queue edits), after a preview or a question. Without a
terminal it prints one frame, as ` + "`sitrep status`" + ` does.

Keys: up, down, j and k move the selection, gg and G go to the first and last session, and
ctrl+d, ctrl+u, pgdn and pgup move by a page; enter jumps to the session's tmux pane or cmux
surface; v or space opens the session in full (e and i open its queue and the inbox there); tab, shift-tab, h, l, left and right switch
tabs; a number from 1 to 9 asks whether to move that inbox item into the selected session
(y or n); a shows or hides ended sessions; s sorts Table, or sizes the Wall tiles; n names the session (an alias); y or
Y copies the session id or its resume command; z snoozes a session until it changes; / filters
the sessions (enter keeps the filter, esc clears it); i opens the inbox and e the selected
session's queue: there o (or a) adds an item (typed), enter (or E) edits the selected one, x
removes one after a y or n (dd; x and delete work too), K and J move it up and down, T moves it to the front, m sends it to
another queue (a list of the inbox and the running sessions: j and k, then enter), P pins the
Objective and U unpins it, and in the inbox t takes the selected item into the session that was
selected, after a y or n, to the end of its queue; p prunes sessions that are gone, after a
preview and a y or n; r refreshes; ? lists the keys on every screen; q goes back from a screen
and quits from the tabs, and ctrl+c quits from anywhere.

--once prints one frame as plain text and exits, --quit-on-jump leaves the dashboard after
a good jump, for a tmux popup, --read-only (or SITREP_TUI_READONLY) turns off p, z and every
other key that changes state, and --no-bell (or SITREP_TUI_BELL=0) stops the bell that rings
when a session starts needing you. --mouse (or SITREP_TUI_MOUSE=1) makes a click select a row and
a double click jump, as enter does; the terminal then selects text only with shift or option held.
`,
	"status": `usage: sitrep status [--tab cards|table|wall] [--width n] [-a]

Print one frame of the dashboard as plain text: for a script, a pipe or a quick look. -a includes
ended sessions.
`,
	"setup": `usage: sitrep setup [--remove]

Put the sitrep block on Claude Code's status line, keeping a status line that is already there
(such as Claude HUD) above it, and link sitrep into BIN_DIR (default ~/bin). It writes
~/.claude/settings.json, after a backup, and changes nothing when that cannot be done safely.
--remove takes it off again and restores the status line it wrapped. State in ~/.claude/sitrep is
kept. The plugin brings the hooks and the skill.
`,
	"hook": `usage: sitrep hook

Handle one Claude Code hook event, read as JSON from standard input: SessionStart,
UserPromptSubmit, Stop or SessionEnd. It keeps each session's Objective, Progress, Now and state,
answers the + commands (+pin, +, +inbox, +take) and starts the summarizer. It never fails and
prints nothing but a block answer. The plugin runs it; you do not.
`,
	"statusline": `usage: sitrep statusline

Print the sitrep block of the status line for the session Claude Code describes as JSON on
standard input, after the output of the status line saved by setup, if any. Claude Code runs it.
It also records how full the session's context window is (in the session's folder), for the CTX
column of the dashboard and the context field of ls --json.
`,
	"version": `usage: sitrep version

Print the version of this program. It matches the version of the plugin that shipped its source.
`,
	"completion": `usage: sitrep completion bash

Print a bash completion script. Add this to ~/.bashrc:

  eval "$(sitrep completion bash)"

It completes command names, flags, the values of --tab, and session ids (the shortest unique prefix
of at least four characters, and inbox where the command takes it).
`,
	"migrate": `usage: sitrep migrate [--dry-run]

Move the state of sitrep 0.4 and before, which kept every file of every session in ~/.claude/sitrep
(<id>.txt, .<id>.state and so on), into a folder for each session, ~/.claude/sitrep/sessions/<id>/.
Run it once, after /sitrep:setup has installed 0.5 or newer. It moves only files with those names,
keeps their modification times, never overwrites a file that is already in place, and deletes the
.prompt files that nothing reads any more. It is safe to run again. --dry-run says what it would do.
`,
	"help": `usage: sitrep help [command]

List the commands, or show the details of one. ` + "`sitrep <command> --help`" + ` shows the same.
`,
}

// helpAll is the list printed by `sitrep help`. Commands appear here as they are ported.
const helpAll = `usage: sitrep <command> [args]

  ls [id]                    list running sessions, or one queue
  add <id> <text>            append an item to a queue
  alias [<id> <name>]        name a session (--off removes the name), or list the names
  edit <id> <n> <text>       replace the text of an item (--item <old text> guards it)
  rm <id> [n]                remove an item and print it (--item <text> guards it)
  done <id> [n]              the same as rm, for an item that is finished
  top <id> <n>               move an item to the front of a queue
  reorder <id> <from> <to>   move an item to another position
  move <from> <to> [n]       hand an item to another queue (--item <text> guards it)
  pin <id> <text>            fix a session's Objective
  unpin <id>                 let the summarizer rewrite the Objective
  snooze [--off] <id>        mute the needs-you state of a session until it changes
  prune                      clear out sessions that are gone
  log                        show the history of changes, when SITREP_LOG is on
  dash                       live full-screen dashboard of the sessions and their queues
  status                     print one frame of the dashboard as text
  setup [--remove]           put sitrep on the status line, or take it off
  migrate [--dry-run]        move state of version 0.4 and before into a folder per session
  version                    print the version of this program
  completion bash            print a bash completion script
  help [command]             this list, or the details of one command

An <id> is a unique prefix of a session id, or "inbox".
Add --help to any command for its details.
`
