package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
)

// spec says what a command accepts, for completion: its flags, the flags that take a value (and the values they may have), and which
// positional arguments are queue ids.
type spec struct {
	flags    []string
	values   map[string][]string // flags that take a value; a nil list means any text, which is not completed
	ids      []int               // the positions of the arguments that are a session id or the inbox
	noInbox  bool                // the ids are sessions only
	noIDWith string              // a flag that means there is no id (ls --json)
}

var specs = map[string]spec{
	"ls":      {flags: []string{"-a", "--all", "-v", "-n", "--json", "--full"}, values: map[string][]string{"-n": nil}, ids: []int{0}, noIDWith: "--json"},
	"add":     {ids: []int{0}},
	"edit":    {flags: []string{"--item"}, values: map[string][]string{"--item": nil}, ids: []int{0}},
	"rm":      {flags: []string{"--item"}, values: map[string][]string{"--item": nil}, ids: []int{0}},
	"top":     {flags: []string{"--item"}, values: map[string][]string{"--item": nil}, ids: []int{0}},
	"reorder": {flags: []string{"--item"}, values: map[string][]string{"--item": nil}, ids: []int{0}},
	"move":    {flags: []string{"--item"}, values: map[string][]string{"--item": nil}, ids: []int{0, 1}},
	"pin":     {ids: []int{0}, noInbox: true},
	"unpin":   {ids: []int{0}, noInbox: true},
	"snooze":  {flags: []string{"--off"}, ids: []int{0}, noInbox: true},
	"prune":   {flags: []string{"-y", "--yes", "-k", "--keep", "--dry-run"}},
	"watch": {flags: []string{"--tab", "--interval", "--once", "--read-only", "--no-bell", "--quit-on-jump", "--width"},
		values: map[string][]string{"--tab": {"watch", "top"}, "--interval": nil, "--width": nil}},
	"status":     {flags: []string{"--tab", "--width", "-a", "--all"}, values: map[string][]string{"--tab": {"watch", "top"}, "--width": nil}},
	"setup":      {flags: []string{"--remove"}},
	"completion": {},
	"version":    {},
	"help":       {},
}

// hidden commands are run by Claude Code or by sitrep itself, and are not offered.
var hidden = map[string]bool{"hook": true, "hook-summarize": true, "update-check": true, "statusline": true, "__complete": true}

func commandNames() []string {
	var out []string
	for name := range commands {
		if !hidden[name] && !strings.HasPrefix(name, "-") {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// complete answers one completion request. words are what has been typed after `sitrep`; the last one is the word being completed, and is
// empty when the cursor follows a space. It returns every candidate that could go there: the shell keeps the ones that match.
func (e Env) complete(ctx context.Context, words []string) []string {
	out := e.candidates(ctx, words)
	if len(out) == 0 {
		return nil
	}
	out = append([]string(nil), out...)
	sort.Strings(out)
	return out
}

func (e Env) candidates(ctx context.Context, words []string) []string {
	if len(words) <= 1 {
		return commandNames()
	}
	cmd, rest := words[0], words[1:]
	sp, ok := specs[cmd]
	if !ok {
		return nil
	}
	cur, before := rest[len(rest)-1], rest[:len(rest)-1]
	if cmd == "help" {
		if len(before) == 0 {
			return commandNames()
		}
		return nil
	}
	if cmd == "completion" {
		if len(before) == 0 {
			return []string{"bash"}
		}
		return nil
	}
	// the word before the cursor may be a flag that wants a value
	if len(before) > 0 {
		if vals, takes := sp.values[before[len(before)-1]]; takes {
			return vals
		}
	}
	if strings.HasPrefix(cur, "-") {
		return sp.flags
	}
	// the position of the word among the arguments that are not flags or their values
	pos, noID := 0, false
	for i := 0; i < len(before); i++ {
		w := before[i]
		if w == sp.noIDWith {
			noID = true
		}
		if _, takes := sp.values[w]; takes {
			i++
			continue
		}
		if strings.HasPrefix(w, "-") {
			continue
		}
		pos++
	}
	if noID {
		return nil
	}
	for _, p := range sp.ids {
		if p == pos {
			return e.completeIDs(ctx, !sp.noInbox)
		}
	}
	return nil
}

// completeIDs lists the sessions by the shortest prefix of their id, at least four characters, that no other session shares: the way the
// status line shows them and the way sitrep resolves them. The inbox comes with them where it is a queue the command takes.
func (e Env) completeIDs(ctx context.Context, withInbox bool) []string {
	snap := e.builder().Build(ctx, snapshot.Options{All: true, Cap: 0})
	ids := make([]string, 0, len(snap.Sessions))
	for _, s := range snap.Sessions {
		ids = append(ids, string(s.ID))
	}
	out := shortestPrefixes(ids)
	if withInbox {
		out = append(out, "inbox")
	}
	return out
}

// shortestPrefixes gives each id the shortest prefix of at least four characters that is the prefix of no other id.
func shortestPrefixes(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		n := min(len(id), 4)
		for n < len(id) && sharedPrefix(id[:n], id, ids) {
			n++
		}
		out = append(out, id[:n])
	}
	return out
}

func sharedPrefix(prefix, self string, ids []string) bool {
	for _, o := range ids {
		if o != self && strings.HasPrefix(o, prefix) {
			return true
		}
	}
	return false
}

func cmdComplete(e Env, args []string) error {
	if len(args) == 0 {
		args = []string{""}
	}
	for _, c := range e.complete(context.Background(), args) {
		fmt.Fprintln(e.Stdout, c)
	}
	return nil
}

// bashScript is what `sitrep completion bash` prints. It works in bash 3.2 (the one macOS ships) and later, and asks the program itself what
// to offer, so it can never be out of date.
const bashScript = `# bash completion for sitrep. Add to ~/.bashrc:  eval "$(sitrep completion bash)"
_sitrep() {
  local cur=${COMP_WORDS[COMP_CWORD]} out
  out=$("${COMP_WORDS[0]}" __complete "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null)
  local IFS=$'\n' # after the call: bash 3.2 joins the words of the array expansion with it
  COMPREPLY=( $(compgen -W "$out" -- "$cur") )
}
complete -F _sitrep sitrep
`

func cmdCompletion(e Env, args []string) error {
	if len(args) != 1 || args[0] != "bash" {
		return usageError("usage: sitrep completion bash")
	}
	fmt.Fprint(e.Stdout, bashScript)
	return nil
}
