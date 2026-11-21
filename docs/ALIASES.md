# Session aliases, version 0.6.0

Status: built. Designed and reviewed once. Changes from the review: the name of an ended session is never taken over, an id wins over an alias in `Resolve`, a name read from disk is validated before use, the Label is used in every message a person reads, and the status line keeps the Short beside the alias.

## Purpose

A session is named by the first four characters of its id (`c5d0`), which are easy to mistype and impossible to remember. This document gives a session an optional name chosen by the user (`sitrep`, `mbe`), usable anywhere an id prefix is, and shown where the four characters are shown today.

## Terms

1. Alias -- A name the user gives a session, stored in the file `alias` of the session folder.
2. Short -- The first four characters of the id, which `ls` shows and `--json` carries as `short`. It does not change.
3. Label -- What a person sees for a session: its alias when it has one, and its Short otherwise.

## The Alias

1. A name matches `^[a-z][a-z0-9-]{0,31}$`: lowercase, starting with a letter, at most 32 characters. `inbox` is reserved. A name that is the start of any existing session id is refused, and the message says so, as a name made of the letters `a` to `f` often is. This check is advice for the user, not what keeps the names apart: `Resolve` does that.
2. The file `sessions/<id>/alias` holds the name and a newline, mode `0600`, written the way every state file is (`Store.WriteFile`). A session without the file has no alias, and an alias without a `notes` file makes no session.
3. A reader of the file (`Store.Alias(id)`, the only one) takes it through `state.ReadFile` with a cap of 64 bytes, uses the first line, and treats anything that does not match the name grammar as no alias. A link, a pipe or a huge file is no alias, and no code prints the text of the file, only a name that passed the grammar. The dashboard decodes `ls --json` as untrusted and cleans the field again.
4. A name belongs to one session. `sitrep alias` takes the folder's queue lock (`state.Lock` on `.queue.lock`, which `Store` shall expose beside `Queue`'s own use of it, so that one lock is the lock) and looks at every session folder before it writes. A name that another session holds is refused, whatever the state of the holder, and the message names the holder and the way to free the name (`sitrep alias <holder> --off`). An ended session may be resumed, so its name is not taken over.
5. `sitrep alias <id> --off` removes only the file. Nothing in sitrep removes a session folder today, so nothing else frees a name.

## The Command

1. `sitrep alias <id> <name>` sets the alias, and prints `c5d0 is now called sitrep`. Setting the name a session already has changes nothing and says so.
2. `sitrep alias <id> --off` removes the alias and prints what it removed. With no alias it says so and exits 0.
3. `sitrep alias` with no arguments lists the aliases, one per line, `name  c5d0  directory`, sorted by name.
4. The id may be an id prefix or an alias. The inbox has no alias.

## Resolving a Name

`Store.Resolve` takes one more step, and is still the only place an id is resolved:

1. `inbox` is the inbox.
2. A word that is a prefix of an id is resolved as before: one match is the session, and several are an error that says how many. A word that matches an id is never read as an alias.
3. Only when no id starts with the word is it looked up as an alias, as an exact match and never as a prefix, so `sit` does not find `sitrep`. Two sessions with the same alias (a hand-edited file, or two writers that raced) make the word ambiguous, and the error says so.

An id always beats an alias, so what the output shows for a session (`cafe9d…` as `cafe`) is never taken from it by a name that a user chose earlier or that a hook's new session happens to start with. The grammar check when a name is set keeps this from being noticed in the common case, and the order keeps it harmless in the rare one: a name that a later id starts with is reached by its alias only until that id exists, and then by a longer word. Claude Code's ids are hexadecimal with dashes, so a name that contains a letter after `f` can never start one.

## Where It Shows

1. `Label` is one function beside the Short (`snapshot.Session.Label()`, and `queue.Label` for the code that has only an id and a store). Every place a person reads a session's name uses it, and every place that passes a name to a program passes the full `id`. The dashboard never hands an alias to a subprocess.
2. `sitrep ls` and the dashboard draw the Label in the first column. The column is as wide as the longest Label in the whole snapshot (not only the rows in view, or it would move while a filter is typed), at least four and at most 16, measured with `ansi.StringWidth`, and a longer alias is cut with `…`. The heading `ID` stays. The width arithmetic that assumes four columns (`avail` in `render`, `dw` in the dashboard) takes the width as an argument.
3. The dashboard's titles, its detail view, the jump message, the queue move target, `X has ended`, and its filter (which matches the alias as well as the id) use the Label.
4. The status line reads `Queue [id c5d0 sitrep]: …`: the Short first, as it is today, and the alias after it when there is one. Claude reads its own session from this line and from the scratchpad path, and a name that later moves would mislead it.
5. `ls --json` keeps `id` and `short`, and gains `alias`, a string that is empty without one. A new field needs no new version of the document.
6. The messages of the queue commands (`queued on sitrep`) use the Label, so a user who types aliases reads them back. The typed errors of the `queue` package (`queue changed; run 'sitrep ls c5d0'`) keep the Short, as they hold no store: the Short always works as an id.
7. Completion offers aliases beside the shortest unique id prefixes, in the places where it offers ids (`alias` itself takes an id in position 0, no inbox, and the flag `--off`). `shortestPrefixes` stays on ids only.
8. The `sitrep-queue` skill says that a session can be named by its alias.

## What Does Not Change

The hooks, the summarizer, the queue files, `migrate` and `prune` do not read the alias. `migrate` has nothing to move, as no earlier layout has the file. State of 0.5.x works unchanged, and the dashboard of an older program ignores the new JSON field.

## Tests

1. The name grammar, with the reserved word, a prefix of a live id, a hostile name (a path, a control character, a very long one) and the empty string.
2. Setting, replacing, removing, a name held by a running session, by an ended one (refused as well), and the same name set twice. An alias named like a command or a flag word (`off`, `add`) works.
3. Two goroutines set one name on two sessions under `-race`: one wins. No `.sitrep.*` temporary file is left, and the file has mode `0600`.
4. `Resolve`: an exact alias, a word that matches an id and an alias (the id wins), a prefix of an id, two holders, an alias that matches nothing, and the inbox. A session created after the alias, with an id that starts with it.
5. A hostile alias file: an escape, a C1 character, a link, a pipe, a megabyte, and a file with no `notes` beside it. Each reads as no alias and prints nothing from the file.
6. The width of the columns in `ls` and the dashboard, with no alias, a short one, the longest, one past the cap, and a filter that hides the widest; the existing layout tests (`render`, `ls`, the dashboard views) are updated for the new width rule.
7. The status line with and without an alias, `--json` with the field (and a hostile value cleaned on the dashboard side), and the dashboard passing `id` and never the alias to a command.
8. Completion of an alias in each id position, and `alias` in the table of commands.
9. The differential tests keep passing, as no alias exists in any of their scenarios.

## Rollback Cost

A user who goes back to 0.5.x keeps a file the old program ignores, and the sessions are named by their Short again. Nothing else needs undoing.
