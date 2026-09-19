// Package discover locates the live session transcript for a working directory.
package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Slug maps a working directory to its transcript folder name. Both '/' and
// '.' become '-', so /home/you/.config -> -home-you--config.
func Slug(cwd string) string {
	r := strings.NewReplacer("/", "-", ".", "-")
	return r.Replace(cwd)
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

// Resolved describes where a transcript was found.
type Resolved struct {
	CWD     string   // the directory whose session this is
	Dir     string   // project dir holding the transcripts
	Path    string   // newest transcript
	Tried   []string // project dirs checked, in order
	Climbed bool     // true when found above the starting directory
}

// Resolve finds the session for cwd, walking up parent directories when the
// starting one has no transcripts. Running ccxray from a subdirectory of the
// project Claude Code is working in is the common case, so it must work.
func Resolve(cwd string) (Resolved, error) {
	r := Resolved{CWD: cwd}
	dir := filepath.Clean(cwd)
	home, _ := os.UserHomeDir()

	for {
		pd := ProjectDir(dir)
		r.Tried = append(r.Tried, pd)
		if p, err := Newest(pd); err == nil && p != "" {
			r.CWD, r.Dir, r.Path = dir, pd, p
			r.Climbed = filepath.Clean(cwd) != dir
			return r, nil
		}
		parent := filepath.Dir(dir)
		// stop at the filesystem root, and never climb above $HOME
		if parent == dir || dir == home || parent == "." {
			return r, fmt.Errorf("no Claude Code transcripts for %s or any parent", cwd)
		}
		dir = parent
	}
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
