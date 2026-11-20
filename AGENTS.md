# AGENTS.md -- claude-sitrep

Guidance for agents working in this repository. The commit and comment rules are adapted from `savannah-dbagent`.

## Linting

After editing any shell script, run `make lint`. It runs `bash -n` and `shellcheck` over every script listed in the `Makefile`, which CI also runs. Add a new script to `SCRIPTS` there.

## Commit Messages

Keep them short, in Simple Technical English, using the project's own terms.

Do not retell how the commit came to be. It will be read in its final form, not as a sequence of intermediate steps. Do not list testing or verification performed.

Do not add `Co-authored-by` trailers or similar attribution metadata, and do not add "Generated with" lines to commits or pull request descriptions.

If you have GitHub push access, check with the user before pushing.

## Code Comments

Take an implicit simplify pass over every comment you write.

Comment only what is non-obvious, or to flag a caveat a reader would otherwise assume away. Do not narrate approaches that were tried and discarded; readers see the final form, not the path to it, and that belongs in the commit message.

Do not explain the language. Comment the decision instead.
