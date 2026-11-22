# AGENTS.md -- claude-sitrep

Guidance for agents working in this repository. The commit and comment rules are adapted from `savannah-dbagent`.

## Checking

sitrep is one Go program. After any change, run `make test`, which CI also runs: `gofmt`, `go mod tidy -diff`, `go vet` and `go test -race`. Add a test for every behavior or bug fix. The plugin cache is keyed by `version` in `.claude-plugin/plugin.json`, so bump it for every change users should receive. `claude plugin validate .` checks the manifests.

The scenario tests in `internal/cli` (with the harness in `internal/clitest`) run the built program in a throwaway `$HOME` through scenarios of commands and hook events, and compare what it printed and left in the state folder with the golden files in `internal/cli/testdata`. `go test ./internal/cli -update-golden` rewrites them: review the diff as carefully as a change of code. They began as differential tests against the last bash version, the git tag `bash-final`, which nothing uses now.

## Invariants

1. Text from prompts, transcripts and models is untrusted terminal input. Strip control (Cc, including C1) and format (Cf) characters before storing it and before printing it. Filtering only ASCII bytes is not enough. `textsafe.Text` can only be made by `Clean`, `Sane` or `Strip`, and the code that renders or encodes takes nothing else. The upstream status line is the user's own command and is passed as it is.
2. State files are owner-only: explicit modes (`0600` files, `0700` folders) and a unique `os.CreateTemp` name in the state folder for every temporary file, published by rename. A `Umask` call does not reach tests, so never rely on it alone.
3. A session id becomes part of a file name, so it is a `state.ID`, made only by `state.ParseID` or `Store.Resolve`: `[A-Za-z0-9_-]` and nothing else, and `inbox` is reserved for the queue of no session. An alias (the name a user gives a session) is read only through `Store.Alias`, which accepts a first line that passes `state.ValidAlias` from a regular file that is not a link, and an id always wins over an alias in `Resolve`. A command gets the `id`, never the alias. The context record is read only through `Store.Context` (a regular file that is not a link, four numbers in range) and written only by `Store.RecordContext`, for a session that already has notes, so an unknown id makes no folder. Read transcripts only through `internal/transcript`: regular files under `<config>/projects/`, opened without following a link. Read state files only through `state.ReadFile`, which refuses what is not a regular file, a link included, and never blocks on a pipe. `state.ReadFileFollow` is for the three kinds of file that are not ours and may be links on purpose: `sitrep.conf` and `settings.json` (a dotfiles manager links them) and the registry files of Claude Code.
4. Hooks must never fail, hang or swallow a user's prompt: exit 0 on any error, block only after the action the user asked for has succeeded, and take locks with a bounded wait.
5. `sitrep setup` changes nothing until the settings edit has been checked and has succeeded, and says what it found. It touches only what it owns: the `statusLine` key (merged into, never replaced), our line in `sitrep.conf` and its block of commented examples (`conf.PutExamples`, which keeps the user's own lines), the program at `~/.claude/sitrep/bin/sitrep`, a link to it, and the move of our own old-layout state files into `sessions/` (`internal/migrate`, also `sitrep migrate`; it links and never overwrites, and no hook calls it). It writes through a symlinked `settings.json`, backs it up first (owner-only, never over an earlier backup), and checks that nothing else in the file changed. It stops, changing nothing, when it finds an install from a checkout of the old bash version. The plugin's hooks live in `hooks/hooks.json`, never in settings. `rm` only single files by their own path, and `chmod` only to tighten, never through a link: no code may recurse over the state folder or its `bin`.
6. Our status line command ends in the tag `# claude-sitrep`, which is how any clone's wrapper is recognised as ours.
7. The summarizer runs isolated (empty working directory, no tools, no MCP, no settings, a time limit, its own process group that the limit kills whole), and a summary that arrives after a newer prompt is dropped.
8. Changes to a queue take the folder's queue lock (`state.Lock`), so an append, a removal, a move and a reorder cannot lose one another's work.
9. The update check (`internal/update`) is the only network use besides the summarizer. A hook only claims it (at most once a day) and starts the detached `update-check`; the child has a deadline and its own process group, and the status line only reads the record it wrote. A tag is shown only after `update.Parse` has turned it into numbers, and nothing is downloaded or installed. `SITREP_CHECK_UPDATES=0` turns it off, and the differential tests set it.

## Commit Messages

Keep them short, in Simple Technical English, using the project's own terms.

Do not retell how the commit came to be. It will be read in its final form, not as a sequence of intermediate steps. Do not list testing or verification performed.

Do not add `Co-authored-by` trailers or similar attribution metadata, and do not add "Generated with" lines to commits or pull request descriptions.

If you have GitHub push access, check with the user before pushing.

## Code Comments

Take an implicit simplify pass over every comment you write.

Comment only what is non-obvious, or to flag a caveat a reader would otherwise assume away. Do not narrate approaches that were tried and discarded; readers see the final form, not the path to it, and that belongs in the commit message.

Do not explain the language. Comment the decision instead.
