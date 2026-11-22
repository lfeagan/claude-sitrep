# Remote machines over ssh

sitrep keeps its state on one machine and has no server. To watch the sessions of another machine, or to put an item on one of its queues, use `ssh` and the commands that are already there. Nothing needs installing beyond sitrep on the remote machine and an ssh login you already have. We looked at sharing the state folder through a synced drive and dropped it, because a hook cannot write over a network without waiting (see `SHARED-STATE.md`).

## Where `sitrep` is

A command run by `ssh host <command>` does not start a login shell, so `sitrep` is often not on `PATH`. Use the full path, `~/.claude/sitrep/bin/sitrep`, which is where `/sitrep:setup` puts the program. The examples below say `sitrep` for short. Add `~/.claude/sitrep/bin` to the `PATH` that a non-interactive shell reads (`~/.zshenv` for zsh, or the top of `~/.bashrc` for bash) to make them work as written.

## Watch

```
ssh devbox sitrep ls
ssh devbox sitrep ls -v
ssh devbox sitrep ls --json | jq '.sessions[] | {id, alias, state, queue: .queue.count}'
```

`ls --json` is the versioned format, and its text has had control and format characters removed on the remote machine, so it is safe to print. Check `version` before you rely on a field. For the dashboard on the remote machine, `ssh -t devbox sitrep dash` runs it in your terminal, with `--read-only` if you only want to look. `enter` jumps only in a terminal that the dashboard itself runs in, so on a remote machine it will not move your local window.

## Queue

```
ssh devbox "$(printf '%q ' sitrep add inbox 'look at the slow dashboard query')"
ssh devbox "$(printf '%q ' sitrep add api 'fix the flaky checkout test (CI)')"
ssh devbox sitrep move inbox api 1
```

The remote shell reads the command line again, so text with spaces or characters such as `( ) ; &` has to be quoted for it. `printf '%q '` does that for any text. An id may be a name given with `sitrep alias`, as it is on the remote machine, so `api` here is the remote machine's `api`.

## Many machines

A loop is enough for a few hosts:

```
for h in devbox laptop buildbox; do echo "== $h"; ssh -o ConnectTimeout=3 "$h" sitrep ls; done
```

Keep a connection open with `ControlMaster auto`, `ControlPath` and `ControlPersist` in `~/.ssh/config`, so that each run does not pay for a new login.

## Cautions

- **The host has to be awake and reachable.** There is no offline view. A host that does not answer shows as an ssh error, not as an empty list.
- **Fixed commands only.** Run `sitrep` commands with the arguments you mean. Do not build a command line from text that came from an issue, a prompt or a model.
- **Write access is login access.** Anyone who can ssh in as you can change your queues, as they could on your own machine.
- **Not tried here.** The commands and the quoting were checked on one machine, but the ssh round trip itself was not, since there was no second host to log in to.

## If this is not enough

A configured list of ssh hosts could feed extra rows into `sitrep dash`, through the same snapshot as `ls --json`, with the keys that change things running `ssh host sitrep ...`. It would be a third network use besides the summarizer and the update check, so invariant 9 would have to change on purpose. We wait to see whether this recipe is enough first.
