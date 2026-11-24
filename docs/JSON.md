# The JSON format

The contract of `sitrep ls --json`, for programs. The short version is in the [README](../README.md#for-programs).

`sitrep ls --json [-a] [-n items | --full]` prints one JSON document that describes the running sessions, and is meant for a program that draws its own view. `-a` includes ended sessions, `-n` caps the items listed in each queue (default 5, or `SITREP_QUEUE_ITEMS`), and `--full` lists them all. It follows these rules:

1. It exits 0 and prints a valid document even with no sessions (`"sessions": []`). On an internal error it exits non-zero, prints a message on stderr and prints nothing on stdout.
2. Every text field is a single line with no control or format characters, so it is safe to draw. A program should still strip them again.
3. `version` is an integer that changes only when a field is removed or its meaning changes. New fields can appear without a new version, so a program should ignore fields it does not know and refuse a version higher than it was written for.
4. The sessions are those active in the last day, and any session that is still running however long it has been quiet, newest first by when their state last changed.

The top level holds these fields:

| Field | Meaning |
|---|---|
| `version`, `generated` | The contract version, and the time of the listing in epoch seconds. |
| `registry` | `ok` when liveness came from Claude Code's session registry, `unavailable` when it fell back to the hook states. |
| `settings` | `queue_items`, `highlight_seconds`, `highlight_sgr`, `theme` and `colors`, as set in the environment or `sitrep.conf`. |
| `hidden` | `ended` counts the finished sessions left out, and `stranded_items` counts the queue items they still hold. |
| `inbox` | `count` is the true number of items, and `items` holds the first ones, up to the cap (all of them with `--full`). |
| `sessions` | One object per session, below. |

Each session has these fields:

| Field | Meaning |
|---|---|
| `id`, `short`, `alias` | The full session id, its first four characters, and the name the user gave the session (empty without one). Pass `id` to a command, never `alias`. |
| `running` | False for an ended session, which appears only with `-a`. |
| `state` | `working`, `waiting`, `ended`, `running` (alive in the registry although the hooks say ended), or `-` (no state recorded). |
| `state_label`, `age` | The text the listing shows, such as `working 1m` or `idle? 25m`, and the seconds since the state changed. |
| `state_changed` | When the state changed, in epoch seconds. |
| `quiet_secs`, `idle` | `quiet_secs` is how long the transcript has been quiet, and null unless the session is working. `idle` is true when that is over ten minutes. |
| `needs_input`, `blocked`, `snoozed` | `needs_input` is true for a running session in the `waiting` state: Claude has stopped, and the next move is the user's. It stays true until the user sends a prompt, unless the session is snoozed. It is also true for a session at a permission dialog, whose `blocked` field is `permission` (the field is absent otherwise) and whose `state_label` starts with `blocked`. `sitrep snooze <id>` mutes it until the session next changes (`snoozed` is then true and `needs_input` false), and `sitrep snooze --off <id>` ends the mute at once. |
| `pid` | The live process of the session, from the registry, or null when it is not known here (another pid space). If a resume left several, the first. A hint only: it can be stale by the time it is used. |
| `pinned`, `highlight` | Whether the Objective is pinned, and whether the session changed within `highlight_seconds`. |
| `attn_faded` | Left out unless true: the session needs you (`needs_input`) but has waited longer than `SITREP_ATTN_FADE` seconds, so the dashboard no longer colours it. |
| `dir`, `cwd` | The last part of the working directory, and the whole path. |
| `objective`, `progress`, `now` | The three lines of the status block. |
| `context` | How full the context window was when the session's status line last drew: `used_percent` (0 to 100), `window` and `tokens` (the size and what is in it), and `at` (when it last drew, in epoch seconds). `null` for an ended session or one that has not said. The share left is `100 - used_percent`. |
| `queue` | `count` is the true number of items, and `items` holds the first ones, up to the cap (all of them with `--full`). |
