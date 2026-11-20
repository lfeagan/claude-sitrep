# AGENTS.md -- claude-sitrep

Guidance for agents working in this repository. The commit and comment rules are adapted from `savannah-dbagent`.

## Linting

After editing any shell script, run `make lint` and `make test`, which CI also runs. Lint runs `bash -n` and `shellcheck` over every script listed in `SCRIPTS` in the `Makefile`, so add a new script there. Tests live in `tests/run.sh` and add a case for every behavior or bug fix.

## Invariants

1. Text from prompts, transcripts and models is untrusted terminal input. Strip control characters before storing it and before printing it.
2. Notes are owner-only: `umask 077` in every script that writes them.
3. A session id becomes part of a file name, so reject anything outside `[A-Za-z0-9_-]`.
4. Hooks must never fail a user's prompt: exit 0 on any error other than an intended block.
5. Scripts run on macOS and Linux, so avoid `stat -f` and `sed -i ''` without a fallback.

## Commit Messages

Keep them short, in Simple Technical English, using the project's own terms.

Do not retell how the commit came to be. It will be read in its final form, not as a sequence of intermediate steps. Do not list testing or verification performed.

Do not add `Co-authored-by` trailers or similar attribution metadata, and do not add "Generated with" lines to commits or pull request descriptions.

If you have GitHub push access, check with the user before pushing.

## Code Comments

Take an implicit simplify pass over every comment you write.

Comment only what is non-obvious, or to flag a caveat a reader would otherwise assume away. Do not narrate approaches that were tried and discarded; readers see the final form, not the path to it, and that belongs in the commit message.

Do not explain the language. Comment the decision instead.
