package state

import "strings"

// legacyFiles maps the extension of a file of the old flat layout (0.4 and before) to the name of the file in a session's folder, and says
// whether the old name starts with a dot.
var legacyFiles = map[string]struct {
	file   string
	hidden bool
}{
	"txt": {"notes", false}, "queue": {"queue", false}, "cwd": {"cwd", false},
	"state": {"state", true}, "count": {"count", true}, "stamp": {"stamp", true}, "pending": {"pending", true},
	"pinned": {"pinned", true}, "snooze": {"snooze", true}, "lock": {"lock", true},
}

// Obsolete is the extension of files the shell version wrote (hidden) and nothing reads.
const obsoleteExt = "prompt"

// Legacy says what a name in the root of the state folder is in the old flat layout: <id>.<ext> for txt, queue and cwd, and .<id>.<ext> for
// the others, for a valid session id. file is the name it has in the session's folder; obsolete says nothing reads the file any more.
type Legacy struct {
	ID       ID
	File     string
	Obsolete bool
}

// ParseLegacy reads a name of the old layout. The id has no dot, so the last dot ends it; the inbox is no session, and a name that is
// anything else is not ours (ok is false).
func ParseLegacy(name string) (Legacy, bool) {
	if name == ".queue.lock" {
		return Legacy{}, false // the lock of the whole folder, which stays in the root, not the lock of a session called queue
	}
	hidden := strings.HasPrefix(name, ".")
	base := strings.TrimPrefix(name, ".")
	i := strings.LastIndexByte(base, '.')
	if i <= 0 {
		return Legacy{}, false
	}
	id, ext := base[:i], base[i+1:]
	if !SessionID(id) {
		return Legacy{}, false
	}
	if ext == obsoleteExt {
		return Legacy{ID: ID(id), Obsolete: true}, hidden
	}
	k, ok := legacyFiles[ext]
	if !ok || k.hidden != hidden {
		return Legacy{}, false
	}
	return Legacy{ID: ID(id), File: k.file}, true
}
