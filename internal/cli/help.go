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
	"help": `usage: sitrep help [command]

List the commands, or show the details of one. ` + "`sitrep <command> --help`" + ` shows the same.
`,
}

// helpAll is the list printed by `sitrep help`. Commands appear here as they are ported.
const helpAll = `usage: sitrep <command> [args]

  add <id> <text>            append an item to a queue
  rm <id> [n]                remove an item and print it (--item <text> guards it)
  top <id> <n>               move an item to the front of a queue
  reorder <id> <from> <to>   move an item to another position
  move <from> <to> [n]       hand an item to another queue (--item <text> guards it)
  pin <id> <text>            fix a session's Objective
  unpin <id>                 let the summarizer rewrite the Objective
  help [command]             this list, or the details of one command

An <id> is a unique prefix of a session id, or "inbox".
Add --help to any command for its details.
`
