# The queue

The short version is in the [README](../README.md#the-queue). This page lists every command and the details.

`sitrep` addresses a session by any unique prefix of its id, as shown in the status line and in `sitrep ls`. `sitrep help` lists the commands, and `sitrep help <command>` or `sitrep <command> --help` shows the details of one.

1. `sitrep ls` lists running sessions with their queue (item count and next item), Objective and Now, and `sitrep ls <id>` lists one queue. `-a` adds ended sessions.
2. `sitrep add <id> <text>` appends an item.
3. `sitrep pin <id> <text>` pins that session's Objective, and `sitrep unpin <id>` lets the summarizer rewrite it again.
4. `sitrep rm <id> [n]` removes item `n` (default 1) and prints it. `sitrep done <id> [n]` is the same command under a name that reads right for finished work; there is no separate done state.
5. `sitrep move <from> <to> [n]` hands an item to another queue. With `--item <text>` it refuses unless item `n` is exactly that text, so a program that showed a list cannot move a different item. `rm`, `top` and `reorder` take the same `--item <text>`, which is checked against the item at `n` (or at `from`).
6. `sitrep edit <id> <n> <text>` replaces the text of item `n` (a spelling fix, say) and keeps its place; it takes the same `--item <old text>` guard.
7. `sitrep alias <id> <name>` gives a session a name, to use wherever an id goes (`sitrep ls sitrep`), and `sitrep alias <id> --off` takes it away; with no arguments it lists the names. A name is 1 to 32 lowercase letters, digits and hyphens, starts with a letter, and is neither `inbox` nor the start of a session id. A name that another session holds is refused, so free it first, or, when that session has ended, move the name with `sitrep alias <id> <name> --take` (the dashboard asks first). An id always wins over a name, and a name must match whole (`sit` does not find `sitrep`). `sitrep ls` and the dashboard show the name in the first column in place of the four characters, and the status line shows `Queue [id c5d0 sitrep]`.
8. `sitrep top <id> <n>` moves item `n` to the front, and `sitrep reorder <id> <from> <to>` moves an item to any position. The status line and `sitrep ls` show the first item as the next one.

Changes to a queue take a lock, so an `add` that lands while a `move` or a `reorder` rewrites the file waits for it, and nothing is lost.

The inbox is a queue that belongs to no session, for work you have not yet matched to one. Address it as `inbox` in any command that takes an id: `sitrep add inbox <text>`, `sitrep ls inbox`, `sitrep rm inbox [n]`, and `sitrep move inbox <id>` when you pick the item up. It has no Objective, so `pin` and `unpin` do not apply. `sitrep ls` prints an `Inbox:` line above the table when it holds anything, and the status line appends `Inbox: N` to the Queue line. To fill it from GitHub or Linear issues, see [INBOX-SOURCES.md](INBOX-SOURCES.md).

Inside a session, four prefixes work on the prompt itself, and none of them is sent to Claude:

1. `+ <text>` queues the text to this session.
2. `+pin <text>` pins the Objective.
3. `+inbox <text>` queues the text to the inbox.
4. `+take [n]` moves item `n` of the inbox (default 1) into this session's queue.

The text forms apply only to a single line of at most 200 characters, and `+take` accepts only an optional number. Anything else goes to Claude as written, so a pasted list, or a prompt such as "+take care of the rest", never gets swallowed. So does a prefix with a leading space, and a `+take` that cannot succeed, such as one on an empty inbox.

## Shell completion

`eval "$(sitrep completion bash)"` in `~/.bashrc` completes command names, flags, the values of `--tab`, and session ids: the shortest prefix of at least four characters that no other session shares, which is how the status line shows them, and `inbox` where the command takes it. The script asks the program itself what to offer, so it cannot go out of date, and it works in the bash 3.2 that macOS ships.
