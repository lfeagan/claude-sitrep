package hook

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
	"github.com/lfeagan/claude-sitrep/internal/transcript"
)

// Deadline is how long the summarizer may take.
const Deadline = 60 * time.Second

// claudeArgs is the summarizer's command line, frozen: no session saved, no slash commands, no tools, no MCP servers, no settings, and a
// spending cap, so a hook can never cost more than a few cents.
var claudeArgs = []string{"-p", "--model", "haiku", "--no-session-persistence", "--disable-slash-commands", "--tools", "",
	"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "", "--max-budget-usd", "0.05"}

// Summarizer runs the Haiku summarizer for one session. It is the detached child of the hook.
type Summarizer struct {
	Hook
	Run   runner.Runner
	Sleep func(ctx context.Context, d time.Duration) // waits for d, or until ctx is done
	Env   []string                                   // the environment of the claude call, without SITREP_SUMMARIZING
	Limit time.Duration                              // how long claude may take; Deadline when zero
}

// Opts say when to run. Wait is the seconds to sleep first (the deferred run); Since is the prompt count the summary is for, and when it is
// empty it is read after the wait.
type Opts struct {
	Wait  int
	Nonce string // names the pending file this run owns; the run stops if another has taken it
	Since string
}

// Do waits if asked, summarizes the end of the transcript, and writes Progress and Now (and the Objective, unless it is pinned). A summary
// made for an older prompt than the newest is dropped.
func (z Summarizer) Do(ctx context.Context, id state.ID, tp string, o Opts) {
	defer func() { _ = recover() }()
	if !state.SessionID(string(id)) || !transcript.Valid(tp, z.ConfigDir) {
		return
	}
	since := o.Since
	if o.Wait > 0 {
		// sleep toward the due time on the wall clock, which a suspended laptop moves past while the process sleeps
		due := z.Clock.Now().Unix() + int64(o.Wait)
		for i := 0; i < 5; i++ {
			rem := min(due-z.Clock.Now().Unix(), int64(o.Wait)+1) // a clock that stepped back must not make it sleep for a day
			if rem <= 0 {
				break
			}
			z.Sleep(ctx, time.Duration(rem)*time.Second)
		}
		if ctx.Err() != nil {
			return // told to stop while waiting: change nothing
		}
		pending := z.Store.Path(id, "pending")
		if o.Nonce != "" {
			if _, nonce, ok := ReadPending(pending); !ok || nonce != o.Nonce {
				return // another Stop took the deferred run over: it does the summary
			}
		}
		// the stamp first, then the pending file, so a Stop in between sees one or the other and never neither
		_ = z.Store.WriteFile(z.Store.Path(id, "stamp"), nil)
		_ = os.Remove(pending)
		since = state.FirstLine(z.Store.Path(id, "count"))
	}
	n := &session{Hook: z.Hook, id: id}
	obj := n.notes().Get(id, "Objective")
	if o.Wait == 0 && placeholder(obj) { // the deferred run passes the Objective as it is
		obj = ""
	}
	recent, ok := transcript.Recent(tp)
	if !ok {
		return // the transcript went away: nothing to summarize, and nothing to invent a summary from
	}
	work, err := os.MkdirTemp("", "sitrep-summary-")
	if err != nil {
		return // a neutral working folder: no project CLAUDE.md, settings or MCP servers
	}
	defer os.RemoveAll(work)
	prompt := "Objective so far: " + obj + "\n\nRecent transcript:\n" + recent +
		"\n\nReply with exactly three lines and nothing else:\nObjective: <a short title for the overall objective of this session, max 80 chars; keep the objective so far if it still fits>\nProgress: <what has been accomplished so far, max 120 chars>\nNow: <what is happening or awaiting the user right now, max 120 chars>"
	limit := z.Limit
	if limit <= 0 {
		limit = Deadline
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	out, err := z.Run.Run(ctx, runner.Cmd{Name: "claude", Args: claudeArgs, Dir: work, Env: append(append([]string(nil), z.Env...), "SITREP_SUMMARIZING=1"), Stdin: strings.NewReader(prompt), NewGroup: true})
	if ctx.Err() != nil || (err != nil && len(out) == 0) {
		return // out of time (or cancelled): half an answer is not used
	}
	o2, p, nw := field(string(out), "Objective"), field(string(out), "Progress"), field(string(out), "Now")
	if p == "" || nw == "" {
		return
	}
	// a prompt submitted while we were summarizing makes this summary stale: drop it, the next Stop redoes it
	if state.FirstLine(z.Store.Path(id, "count")) != since {
		return
	}
	o3 := textsafe.Sane(o2, 100).String()
	if _, err := os.Stat(z.Store.Path(id, "pinned")); err == nil || o3 == "" {
		o3 = n.notes().Get(id, "Objective")
	}
	_ = n.write(o3, textsafe.Sane(p, 140).String(), textsafe.Sane(nw, 140).String())
}

// field is the text after the first line that starts with "<key>:" and the spaces after it.
func field(out, key string) string {
	for _, l := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(l, key+":"); ok {
			return strings.TrimLeft(rest, " ")
		}
	}
	return ""
}
