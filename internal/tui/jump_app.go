package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// iTerm2 and Terminal.app are driven by osascript. Both give each session or tab its tty, and sitrep knows the tty of a session's process,
// so the lookup needs no walk up the process tree.

// ttyRe is the one spelling of a terminal that osascript is given. The tty goes to the script as an argument, never into its text.
var ttyRe = regexp.MustCompile(`^/dev/ttys[0-9]+$`)

// termApp is a scriptable terminal application.
type termApp struct {
	Name   string // for messages
	Bundle string // the bundle id, which names the application whatever the user renamed it
	script string
}

// The scripts print "found" after they have selected the window, tab and session that has the tty. They look only at a running application,
// since `tell` would otherwise start it.
var (
	iterm2 = termApp{Name: "iTerm2", Bundle: "com.googlecode.iterm2", script: `on run argv
	set target to item 1 of argv
	if application id "com.googlecode.iterm2" is not running then return ""
	tell application id "com.googlecode.iterm2"
		repeat with w in windows
			repeat with t in tabs of w
				repeat with s in sessions of t
					if (tty of s) is target then
						tell w to select
						tell t to select
						tell s to select
						activate
						return "found"
					end if
				end repeat
			end repeat
		end repeat
	end tell
	return ""
end run`}
	terminalApp = termApp{Name: "Terminal", Bundle: "com.apple.Terminal", script: `on run argv
	set target to item 1 of argv
	if application id "com.apple.Terminal" is not running then return ""
	tell application id "com.apple.Terminal"
		repeat with w in windows
			repeat with t in tabs of w
				if (tty of t) is target then
					set selected of t to true
					set index of w to 1
					activate
					return "found"
				end if
			end repeat
		end repeat
	end tell
	return ""
end run`}
)

// jumpApp selects the window and tab of app that holds the terminal of process pid. ok is false when the application does not show it.
func jumpApp(ctx context.Context, app termApp, pid int, run runner) (ok bool, err error) {
	out, err := run(ctx, "ps", "-o", "tty=", "-p", strconv.Itoa(pid))
	if err != nil {
		return false, nil // no terminal to look for
	}
	tty := normTTY(out)
	if !ttyRe.MatchString(tty) {
		return false, nil
	}
	out, err = run(ctx, "osascript", "-e", app.script, tty)
	if err != nil {
		return false, appError(app, err)
	}
	return strings.TrimSpace(out) == "found", nil
}

// appError turns a refusal by macOS into what the user has to do. Any other error is passed on.
func appError(app termApp, err error) error {
	if strings.Contains(err.Error(), "-1743") || strings.Contains(strings.ToLower(err.Error()), "not authorized") {
		return fmt.Errorf("macOS did not let this terminal control %s: allow it in System Settings > Privacy & Security > Automation", app.Name)
	}
	return errors.New("could not ask " + app.Name + ": " + err.Error())
}
