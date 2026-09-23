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

// Running from a subdirectory of the project Claude Code is working in must
// still find the session, so every parent is a candidate.
func TestCandidatesClimbToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sub := filepath.Join(home, "work", "proj", "tool")

	got := Candidates(sub)
	want := []string{
		ProjectDir(sub),
		ProjectDir(filepath.Join(home, "work", "proj")),
		ProjectDir(filepath.Join(home, "work")),
		ProjectDir(home),
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Candidates =\n  %v\nwant\n  %v", got, want)
	}
}

// The reported failure: ccxray opened on a stale session and never noticed
// the live one. A transcript last written before launch must be ignored.
func TestFirstSinceIgnoresSessionsFromBeforeLaunch(t *testing.T) {
	d := t.TempDir()
	old := filepath.Join(d, "old.jsonl")
	os.WriteFile(old, []byte("{}\n"), 0o644)
	os.Chtimes(old, nowMinus(3600), nowMinus(3600))

	since := nowMinus(1)
	if dir, p := FirstSince([]string{d}, since); p != "" {
		t.Fatalf("picked %s in %s, want nothing until a session is written", p, dir)
	}

	os.WriteFile(filepath.Join(d, "live.jsonl"), []byte("{}\n"), 0o644)
	if _, p := FirstSince([]string{d}, since); filepath.Base(p) != "live.jsonl" {
		t.Errorf("picked %q, want live.jsonl", p)
	}
}

// A project dir Claude Code has not created yet, and a stale parent, must not
// stop the live session in a later candidate from being found.
func TestFirstSinceSkipsMissingAndStaleDirs(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	stale, live := filepath.Join(root, "stale"), filepath.Join(root, "live")
	os.MkdirAll(stale, 0o755)
	os.MkdirAll(live, 0o755)
	s := filepath.Join(stale, "s.jsonl")
	os.WriteFile(s, []byte("{}\n"), 0o644)
	os.Chtimes(s, nowMinus(3600), nowMinus(3600))
	os.WriteFile(filepath.Join(live, "l.jsonl"), []byte("{}\n"), 0o644)

	dir, p := FirstSince([]string{missing, stale, live}, nowMinus(60))
	if dir != live || filepath.Base(p) != "l.jsonl" {
		t.Errorf("got %s %s, want the live dir", dir, p)
	}
}

func nowMinus(sec int) time.Time { return time.Now().Add(-time.Duration(sec) * time.Second) }
