# Blocked sessions -- showing a session that waits on a dialog as needing you

Status: built for permission dialogs (0.12.0), with the timings below measured on Claude Code in a real permission dialog. The question dialog (`elicitation_dialog`) is not built: we have not seen which event it sends. The hook must be added to `settings.json` by hand on a machine that is wired that way, and `sitrep setup` does not yet say it is missing.

## Claim

A session that has stopped at a permission dialog or a question is waiting for the user, and sitrep should say so. Today `needs_input` is true only after a `Stop` hook has fired, and a session blocked in the middle of a turn has not stopped: it shows `working`, and quietly turns `idle?` after ten minutes. That is the case the dashboard's bell and the Needs You count most need to catch, so we should add a hook for the dialog and mark a blocked session as needing the user.

## Today

`hooks/hooks.json` registers `SessionStart`, `UserPromptSubmit`, `Stop` and `SessionEnd`. The `state` word is `working` after a prompt and `waiting` after `Stop`, and the snapshot sets `needs_input` when the word is `waiting`, unless a snooze matches the time the word last changed.

## What Claude Code Sends

We logged every hook event of a session in the default permission mode, with a throwaway hook that also sampled the transcript's modification time. Times are seconds from the prompt:

| At | Event | Transcript last written |
| --- | --- | --- |
| 0.0 | `UserPromptSubmit` | |
| 1.0 | `PreToolUse` (Write) | 0.1 |
| 1.0 | `PermissionRequest` | 0.1 |
| 1.4 | | 1.4 (one write, 0.3 s after the dialog opened) |
| 7.0 | `Notification`, type `permission_prompt`, "Claude needs your permission" | 1.4 |
| 17.4 | `PostToolUse` (the user had approved) | 1.4, then 17.5 |
| 18.1 | `Stop` | 17.5 |

Thus:

1. `PermissionRequest` fires the moment the dialog opens. `Notification` follows about six seconds later, so it is too late to be the trigger for a permission.
2. The transcript is written once just after the dialog opens and then stays unchanged until the user answers, which is what the snapshot rule needs, with a slack of a few seconds for that first write.
3. In `auto` mode, which the first session of the capture ran in, no dialog opens and neither event fires.
4. `PermissionRequest` carries the `tool_name` and `permission_suggestions` and no `notification_type`. A hook for it can answer the dialog (allow or deny) through its output, so ours must print nothing and exit 0, as the others do.

We have not seen a question dialog (`elicitation_dialog`), so we do not know which event it sends.

## Options

1. Set the `state` word to `waiting` on the notification. It is the smallest change, but nothing ends it: once the user answers the dialog the session works again with no `UserPromptSubmit` and no `Stop`, so it would show `needs you` until the end of the turn. A fix needs a `PostToolUse` hook that sets `working`, and that is a process started for every tool call.
2. Record the block in a file of its own, and end it by what the transcript does. The `state` word stays as it is, the hook runs only when a dialog opens, and nothing runs per tool call, as the snapshot already reads the transcript's modification time to find `idle?`.

Option 2 is the better one. Its cost is one more file and a rule in the snapshot.

## Design

1. `hooks.json` gets a `PermissionRequest` entry, and a `Notification` entry with the matcher `elicitation_dialog` for a question (to be confirmed, see Questions). Both run `sitrep hook` as the others do. The `permission_prompt` notification is not registered, as `PermissionRequest` has said it six seconds earlier.
2. The hook writes `sessions/<id>/blocked`, one line `<epoch> <kind>`, where kind is `permission` or `question` and is chosen by us from the event. The message and the tool input are not stored, as they are text from a model and may hold a path or a command. It is one `WriteFile` and it needs no lock, so it fits the budget and never fails a prompt (invariant 4). It prints nothing, so it never answers the dialog. An event of another kind changes nothing.
3. `UserPromptSubmit`, `Stop`, `SessionStart` and `SessionEnd` remove the file, each by its own path.
4. The snapshot treats a session as blocked when the file is there, the word is `working`, and the transcript has not been written since the block (a slack of three seconds covers the write that follows the dialog by 0.3 seconds). A blocked session has `needs_input` true, `state_label` `blocked 40s`, and a new field `blocked` set to `permission` or `question` (empty otherwise). The README names `needs_input` as the one to use, so a program that ignores `blocked` still works.
5. A snooze keys on the time of the block, so `z` mutes it until the session changes, as it does for `waiting`.
6. When the user answers, the transcript grows within a moment, the rule fails and the session is `working` again. A file left behind by a crash is harmless for the same reason, and the next prompt removes it. A session with no readable transcript keeps the block until the file is removed, as we cannot tell otherwise.
7. The bell rings on the change to `needs_input`, and the Top and Watch tabs already count it, so they need no change beyond showing `blocked` in the state column.

## Where The Hook Has To Be Added

`sitrep setup` never edits hooks (invariant 5), and the plugin's `hooks.json` is the one place they live. A machine that is wired by hand in `settings.json`, as the author's is, needs the entry added there. `sitrep setup` can say that the permission hook is missing from the settings it reads, but it should not add it.

## Rollback

Remove the entry from `hooks.json` and the `blocked` files stop appearing. The snapshot rule is inert without a file, and the field is additive, so a program that never saw it is not affected.

## Questions

1. Which event a question dialog sends. We suspect `PermissionRequest` for the question tool as for the others, which would make the `Notification` entry unnecessary. We should open one in a real session before we register it.
2. Whether three seconds of slack is right. One sample showed a write 0.3 seconds after the dialog, and the user's answer came much later. A user who answers inside the slack leaves the block in place until the next write, which is the `Stop` that removes it, so the cost is a short false `needs you`.
3. Whether the history log should record a block (`session.blocked`). We lean to no, as it is a state of the session and not a change that sitrep made.
