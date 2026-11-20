# AGENTS.md -- claude-sitrep

Guidance for agents working in this repository. The commit and comment rules are adapted from `savannah-dbagent`.

## Linting

After editing any shell script, run `make lint` and `make test`, which CI also runs. Lint runs `bash -n` and `shellcheck` over every script listed in `SCRIPTS` in the `Makefile`, so add a new script there. Tests live in `tests/run.sh` and add a case for every behavior or bug fix.

## Invariants

1. Text from prompts, transcripts and models is untrusted terminal input. Strip control (Cc, including C1) and format (Cf) characters before storing it and before printing it. Filtering only ASCII bytes is not enough.
2. State files are owner-only: `umask 077` in every script that writes them, and a unique `mktemp` name for every temporary file.
3. A session id becomes part of a file name, so reject anything outside `[A-Za-z0-9_-]`. Read transcripts only from regular files under the Claude projects directory.
4. Hooks must never fail or swallow a user's prompt: exit 0 on any error, and block only after the action the user asked for has succeeded.
5. The installers change nothing until the settings merge has succeeded, and say what they found. They touch only what they own. Match our hook by its exact path, merge into `statusLine` and never replace it, write through a symlinked `settings.json`, and leave a `q` that is not ours alone.
6. Our status line command ends in the tag `# claude-sitrep`, which is how any clone's wrapper is recognised as ours. `stat` differs between BSD and GNU. Try `stat -c` first, then `stat -f`, because `stat -f` succeeds with the wrong output on Linux.
7. The summarizer runs isolated (empty working directory, no tools, no MCP, no settings, time limit), and a summary that arrives after a newer prompt is dropped.

## Commit Messages

Keep them short, in Simple Technical English, using the project's own terms.

Do not retell how the commit came to be. It will be read in its final form, not as a sequence of intermediate steps. Do not list testing or verification performed.

Do not add `Co-authored-by` trailers or similar attribution metadata, and do not add "Generated with" lines to commits or pull request descriptions.

If you have GitHub push access, check with the user before pushing.

## Code Comments

Take an implicit simplify pass over every comment you write.

Comment only what is non-obvious, or to flag a caveat a reader would otherwise assume away. Do not narrate approaches that were tried and discarded; readers see the final form, not the path to it, and that belongs in the commit message.

Do not explain the language. Comment the decision instead.
