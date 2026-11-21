# Porting the hook (Phase 5)

Status: draft 2, after review, and built. It follows `docs/GO-PORT.md`.

## Purpose

`sitrep hook` replaces `hooks/sitrep-hook.sh`. It reads one hook event as JSON on standard input and keeps `~/.claude/sitrep/<id>.txt`, the queue and the state word. It must never fail or swallow a prompt (invariant 4), and the Haiku summarizer must run after the hook has returned.

## Shape

1. `cmd/sitrep/main.go` checks `SITREP_SUMMARIZING` first, before the umask, before reading stdin. When it is set and the command is `hook`, it exits 0. The summarizer's own `claude -p` fires our hook, and this stops the loop.
2. `internal/hook.Hook.Run(stdin)` handles one event and returns nothing. A top-level `recover` and a bounded read of stdin (64 MB, so a big paste is still one event; the first JSON value is used and anything after it is ignored) make every failure a silent exit 0.
3. `sitrep hook-summarize` (hidden) is the detached child. The hook starts it with `Setsid`, stdio on `/dev/null`, and does not wait. It runs the debounce sleep when asked, then the summarizer.
4. `internal/notes` gains `Get(id, key)` and `Write(id, obj, prog, now)` for the three-line file, shared with `pin`.

## Event flow (as in the bash)

1. Take `session_id`, `hook_event_name`, `transcript_path`, `cwd`, `prompt`, `source` from the JSON. Strings only. A session id must be `state.SessionID`.
2. The transcript path is used only when it starts with the literal `<config>/projects/`, holds no `/../`, and is a regular file that is not a link (`Lstat`, then opened with `O_NOFOLLOW` and `O_NONBLOCK`). The check is on the raw string, before any cleaning.
3. `SessionEnd`: write the state word `ended`, stop.
4. Every other event writes `<id>.cwd` with the cwd cleaned to 300 characters (empty when there is none).
5. `Objective`, `Progress` and `Now` are read from the notes file. A placeholder Objective (`-`, starts with `<` or `&lt;`, or holds `<command-` or `&lt;command-`) counts as unset.
6. `UserPromptSubmit`: the prompt's trailing newlines are dropped, and the count file is raised by one before anything else, so `+` prompts count too. Then:
   1. `+pin <text>`, `+ <text>`, `+inbox <text>` act only when the text is short (one line, at most 200 characters, not blank), pin or queue it, and answer with `{"decision":"block","reason":...}` and exit 0. A failed write falls through to Claude.
   2. `+take` and `+take N` (N matching `[1-9][0-9]*`) move an inbox item into this session's queue, creating `- - -` notes first when there are none. When the inbox has nothing, the prompt falls through to Claude.
   3. Otherwise: state `working`; the Objective is the old one, else the first real user message of the transcript (cut to 160), else the prompt (cut to 160); `Now` is the prompt (cut to 140); the notes are written.
7. `SessionStart`: state `waiting` unless `source` is `compact`. When there is no Objective and a transcript, seed it from the first message, write `(resumed -- summarizing)`, and summarize if an Objective was found.
8. `Stop`: state `waiting`. With a transcript, the debounce: when the stamp is younger than the window (`SITREP_DEBOUNCE`, default 45 seconds), schedule one deferred run for the end of the window, unless one is already pending; otherwise touch the stamp and summarize now.

## The detached child

1. **Inputs.** The arguments carry only the mode, the session id, the transcript path and the counts, never prompt text, because arguments are visible in `ps`. The child re-reads the notes itself and re-validates the transcript path, since it runs up to a window later than the hook. `Objective so far:` is the notes' Objective with a placeholder blanked, except in the deferred run, which passes it as it is (as the bash did). At write time the Objective is read again, which matters when it is pinned.
2. **Immediate run.** The hook reads the prompt count (`since`) and passes it, so a prompt that arrives between the spawn and the child's start still makes the summary stale.
3. **Deferred run and the pending file.** The pending file holds `<due epoch> <nonce>`. A Stop that finds a pending file whose due time passed more than five seconds ago takes it over by renaming it away (only one of two Stops wins the rename) and creates its own with `O_EXCL`. An older version's file with no time in it counts while it is younger than the window plus five seconds. The child sleeps toward the due time on the wall clock, then checks that the file still holds its nonce and stops if not, so a child that woke after a suspend never acts for a Stop that already replaced it. Then it touches the stamp, and only after that removes the pending file, so a Stop in between sees one or the other.
4. **Debounce arithmetic** is in whole seconds: defer when `now - stamp < window`. `SITREP_DEBOUNCE` unset or empty is 45; a value that is not a number turns the debounce off, as in the bash, where the failed comparison was false; zero or less never defers. A stamp from the future counts as no stamp.
5. **Summarizer.** It runs `claude -p --model haiku --no-session-persistence --disable-slash-commands --tools "" --strict-mcp-config --mcp-config '{"mcpServers":{}}' --setting-sources "" --max-budget-usd 0.05` in an empty temporary directory, with `SITREP_SUMMARIZING=1` added to the environment, a 60 second deadline (`SITREP_SUMMARIZE_TIMEOUT` seconds in tests), and its own process group, which the deadline kills whole. The child traps SIGTERM, SIGINT and SIGHUP and cancels the same way, so a kill of the child does not orphan the claude it started. The prompt is the bash text; every physical line of the recent transcript is cut to 600 characters including its `user: ` or `assistant: ` prefix, then the last 6000 bytes are kept.
6. **Parsing.** The first lines beginning `Objective:`, `Progress:` and `Now:`, with the spaces after the colon dropped. Progress and Now must be non-empty before cleaning. An answer that arrives after the deadline is dropped, where the bash parsed what the killed process had printed. A summary made for an older prompt count is dropped. The Objective is cut to 100, Progress and Now to 140, and a pinned or empty Objective keeps the current one.
7. Writes are atomic. The child never writes to standard output.
8. **Accepted.** An immediate run that is still going when the deferred run finishes can overwrite the newer summary with an older one. The bash had this too, and the next Stop repairs it.

## The plugin refresh

`refresh_plugin` in the bash hook (the plugin link and the status line copy) is not ported. It exists only because of the `plugin` link, which the Go setup does not make (see `docs/GO-PORT.md`). Until the cutover in Phase 8 the bash hook stays the one in use, so nothing is lost.

## Differences from the bash, on purpose

1. A line of a transcript that is not JSON is skipped, where jq stopped at it. A transcript line longer than 16 MB is skipped, and the first-prompt scan reads at most 16 MB, so a prompt never waits on a huge file.
2. The pending file goes stale and has an owner (above), and the stamp is touched before the pending file is removed.
3. The state, count and notes files are written atomically, where the bash truncated and rewrote.
4. A transcript that vanished, or an answer after the deadline, is not summarized or used.
5. Invalid UTF-8 becomes U+FFFD (perl kept the bad bytes), and the transcript tail is cut on a character boundary.
6. `+take` moves the item directly, where the bash ran `sitrep move` and so also failed when the session id was a prefix of another id.
7. A numeric `session_id` or a non-string prompt is not accepted.
8. A stamp from the future is ignored.

## Testing

1. Differential: run the same events through both, with a stub `claude` on `PATH`, and compare the block output, the exit status and the state files (modes included). The summarizer runs under both with the stub, and the test waits for its result.
2. Unit: `Hook` with a fake spawner and a fake runner for the claude call. The child function is called directly.
3. Hostile input: control and bidi characters, C1, a 400 KB prompt, invalid UTF-8, a path-like session id, a transcript that is a link or outside the projects folder, a FIFO as transcript.
4. The hook returns at once: a test starts the real binary with a stub `claude` that sleeps, and the hook process exits in well under the stub's time. A second test shows that the deadline kills a grandchild of claude.
5. The stub `claude` records its arguments, `SITREP_SUMMARIZING` and its whole standard input, so the differential compares the call byte for byte. The non-ASCII cut and tail cases are Go unit tests, since the bash `cut -c` differs between GNU and BSD.

## Rollback cost

None before Phase 8: the bash hook stays the one in use until the cutover.
