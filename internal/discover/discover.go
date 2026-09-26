// Package discover locates the live session transcript for a working directory.
package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Slug maps a working directory to its transcript folder name. As in Claude
// Code, every byte that is not an ASCII letter or digit becomes '-', so
// /home/you/.config -> -home-you--config and "Local Sites" -> Local-Sites.
func Slug(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

func Root() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".claude", "projects")
	}
	return ""
}

func ProjectDir(cwd string) string { return filepath.Join(Root(), Slug(cwd)) }

// Newest returns the most recently modified transcript in dir, which is the
// live session. Returns "" when the directory holds none.
func Newest(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type cand struct {
		path string
		mod  int64
	}
	var cs []cand
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		cs = append(cs, cand{filepath.Join(dir, e.Name()), fi.ModTime().UnixNano()})
	}
	if len(cs) == 0 {
		return "", nil
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].mod > cs[j].mod })
	return cs[0].path, nil
}

// Session resolves an explicit session id to a path within dir.
func Session(dir, id string) string { return filepath.Join(dir, id+".jsonl") }

// Candidates lists the project dirs that may hold the session for cwd, most
// specific first: cwd itself, then each parent up to and including $HOME.
// Running ccxray from a subdirectory of the project Claude Code is working in
// is the common case, so the parents must be watched too.
func Candidates(cwd string) []string {
	var out []string
	dir := filepath.Clean(cwd)
	home, _ := os.UserHomeDir()
	for {
		out = append(out, ProjectDir(dir))
		parent := filepath.Dir(dir)
		// stop at the filesystem root, and never climb above $HOME
		if parent == dir || samePath(dir, home) || parent == "." {
			return out
		}
		dir = parent
	}
}

// samePath compares two cleaned paths the way the filesystem does. Windows
// ignores case, so --project c:\users\you\x must still stop at C:\Users\you.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// FirstSince returns the newest transcript written after since, checking dirs
// in order so the most specific project wins. It returns "" for both when no
// session has been touched yet; a dir that does not exist yet is not an error,
// since Claude Code creates it on the first prompt in a new project.
func FirstSince(dirs []string, since time.Time) (dir, path string) {
	for _, d := range dirs {
		p, err := Newest(d)
		if err != nil || p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.ModTime().After(since) {
			return d, p
		}
	}
	return "", ""
}

// NewestAcross returns the most recently modified transcript in any of dirs,
// and the dir holding it. Unlike FirstSince it does not prefer the most
// specific dir: once attached, a session started from a parent directory — a
// second terminal opened at the editor's workspace root, say — must still be
// able to take over from one that has gone quiet.
func NewestAcross(dirs []string) (dir, path string) {
	var best int64
	for _, d := range dirs {
		p, err := Newest(d)
		if err != nil || p == "" {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if mod := fi.ModTime().UnixNano(); path == "" || mod > best {
			dir, path, best = d, p, mod
		}
	}
	return dir, path
}

// NewestSince returns the most recently written transcript in any of dirs, if
// it was written after since. It is FirstSince with no dir preferred, for
// --all, where the dirs are unrelated projects rather than one's parents.
func NewestSince(dirs []string, since time.Time) (dir, path string) {
	dir, path = NewestAcross(dirs)
	if path == "" {
		return "", ""
	}
	if fi, err := os.Stat(path); err != nil || !fi.ModTime().After(since) {
		return "", ""
	}
	return dir, path
}

// All lists every project dir, for --all. Claude Code creates a project's
// dir on its first prompt there, so the list is read afresh on each call.
func All() []string {
	root := Root()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}

// SubagentDir returns the folder holding forked-skill transcripts for a
// session. Claude Code nests them inside a directory named for the session:
//
//	projects/<slug>/<session-id>/subagents/agent-<agentID>.jsonl
func SubagentDir(sessionPath string) string {
	return filepath.Join(strings.TrimSuffix(sessionPath, ".jsonl"), "subagents")
}

// ForkPath resolves a fork's transcript from the agent id carried in the
// parent's toolUseResult.
func ForkPath(sessionPath, agentID string) string {
	return filepath.Join(SubagentDir(sessionPath), "agent-"+agentID+".jsonl")
}
