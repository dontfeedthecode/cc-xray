package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSlug(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"/home/you/project", "-home-you-project"},
		{"/home/you/.config", "-home-you--config"}, // '.' also becomes '-'
		{"/home/you/project/tool", "-home-you-project-tool"},
	} {
		if got := Slug(c.in); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNewestPicksMostRecent(t *testing.T) {
	d := t.TempDir()
	for _, n := range []string{"old.jsonl", "new.jsonl"} {
		os.WriteFile(filepath.Join(d, n), []byte("{}\n"), 0o644)
	}
	// make new.jsonl distinctly newer
	os.Chtimes(filepath.Join(d, "old.jsonl"), nowMinus(60), nowMinus(60))
	got, err := Newest(d)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "new.jsonl" {
		t.Errorf("Newest = %s, want new.jsonl", filepath.Base(got))
	}
}

func TestNewestIgnoresNonJSONL(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "notes.txt"), []byte("x"), 0o644)
	got, err := Newest(d)
	if err != nil || got != "" {
		t.Errorf("got %q err %v, want empty", got, err)
	}
}

// The regression for the reported failure: running from a subdirectory of the
// project Claude Code is working in must still find the session.
func TestResolveClimbsToParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	project := filepath.Join(home, "work", "proj")
	sub := filepath.Join(project, "tool", "bin")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	pd := filepath.Join(home, ".claude", "projects", Slug(project))
	if err := os.MkdirAll(pd, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(pd, "sess.jsonl"), []byte("{}\n"), 0o644)

	r, err := Resolve(sub)
	if err != nil {
		t.Fatalf("Resolve from subdir failed: %v (tried %v)", err, r.Tried)
	}
	if r.CWD != project {
		t.Errorf("resolved cwd = %q, want %q", r.CWD, project)
	}
	if !r.Climbed {
		t.Error("Climbed should be true when found above the start dir")
	}
	if filepath.Base(r.Path) != "sess.jsonl" {
		t.Errorf("path = %q", r.Path)
	}
	if len(r.Tried) < 3 {
		t.Errorf("expected to try bin, tool, proj; got %v", r.Tried)
	}
}

func TestResolveReportsWhatItTried(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sub := filepath.Join(home, "a", "b", "c")
	os.MkdirAll(sub, 0o755)

	r, err := Resolve(sub)
	if err == nil {
		t.Fatal("expected failure with no transcripts anywhere")
	}
	if len(r.Tried) == 0 {
		t.Error("Tried should list the directories checked")
	}
	for _, d := range r.Tried {
		if !strings.Contains(d, ".claude") {
			t.Errorf("odd candidate %q", d)
		}
	}
}

// Never climb above $HOME. Asserted by the length of the walk: a parent's
// slug is a prefix of its child's, so substring matching gives false hits.
func TestResolveStopsAtHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, "x"), 0o755)

	r, err := Resolve(filepath.Join(home, "x"))
	if err == nil {
		t.Fatal("expected failure")
	}
	// exactly two candidates: <home>/x then <home>, then stop
	if len(r.Tried) != 2 {
		t.Errorf("walk visited %d dirs, want 2 (stop at HOME): %v", len(r.Tried), r.Tried)
	}
	if !strings.HasSuffix(r.Tried[len(r.Tried)-1], Slug(home)) {
		t.Errorf("last candidate %q should be HOME itself", r.Tried[len(r.Tried)-1])
	}
}

func nowMinus(sec int) time.Time { return time.Now().Add(-time.Duration(sec) * time.Second) }
