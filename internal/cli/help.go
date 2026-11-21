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
	"rm": `usage: sitrep rm <id> [n] [--item <text>]

Remove item n (default 1) from a queue and print it. It refuses when the queue
changed after it was read, so it never removes a different item. With --item, it
also refuses unless item n is exactly that text, for a program that showed a list.
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

Mute the "waiting for you" state of one session, so that a program such as sitrep-tui stops
drawing it as needing you. The mute ends by itself when the session changes: the next
prompt, or the end of the next turn. ` + "`" + `--off` + "`" + ` ends it at once. Only the JSON changes: the
` + "`" + `needs_input` + "`" + ` field turns false and ` + "`" + `snoozed` + "`" + ` turns true.
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
	"help": `usage: sitrep help [command]

List the commands, or show the details of one. ` + "`sitrep <command> --help`" + ` shows the same.
`,
}

// helpAll is the list printed by `sitrep help`. Commands appear here as they are ported.
const helpAll = `usage: sitrep <command> [args]

  ls [id]                    list running sessions, or one queue
  add <id> <text>            append an item to a queue
  rm <id> [n]                remove an item and print it (--item <text> guards it)
  top <id> <n>               move an item to the front of a queue
  reorder <id> <from> <to>   move an item to another position
  move <from> <to> [n]       hand an item to another queue (--item <text> guards it)
  pin <id> <text>            fix a session's Objective
  unpin <id>                 let the summarizer rewrite the Objective
  snooze [--off] <id>        mute the needs-you state of a session until it changes
  prune                      clear out sessions that are gone
  help [command]             this list, or the details of one command

An <id> is a unique prefix of a session id, or "inbox".
Add --help to any command for its details.
`
