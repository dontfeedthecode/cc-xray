package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSlug(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"/home/you/project", "-home-you-project"},
		{"/home/you/.config", "-home-you--config"}, // '.' also becomes '-'
		{"/home/you/project/tool", "-home-you-project-tool"},
		{"/Users/you/Local Sites/app/public", "-Users-you-Local-Sites-app-public"}, // and spaces
		{"/home/you/my_app (v2)", "-home-you-my-app--v2-"},
		{`C:\Users\you\cc-xray`, "C--Users-you-cc-xray"}, // as Claude Code names it on Windows
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
	setHome(t, home)
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

// Windows paths ignore case, so a --project typed in lower case must still
// stop at the home directory rather than climb on to the drive root.
func TestCandidatesStopAtHomeInAnyCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive paths are a Windows behaviour")
	}
	setHome(t, `C:\Users\You`)
	got := Candidates(`c:\users\you\work`)
	want := []string{ProjectDir(`c:\users\you\work`), ProjectDir(`c:\users\you`)}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Candidates =\n  %v\nwant\n  %v", got, want)
	}
}

// setHome points os.UserHomeDir at dir, which reads USERPROFILE on Windows
// and HOME everywhere else.
func setHome(t *testing.T, dir string) {
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
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

// --all watches every project dir, including ones Claude Code creates after
// launch, and never a stray file beside them.
func TestAllListsEveryProject(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	root := filepath.Join(home, ".claude", "projects")
	os.MkdirAll(filepath.Join(root, "C--work-a"), 0o755)
	os.WriteFile(filepath.Join(root, "stray.json"), []byte("{}"), 0o644)
	if got := All(); len(got) != 1 || filepath.Base(got[0]) != "C--work-a" {
		t.Fatalf("All = %v, want the one project dir", got)
	}
	os.MkdirAll(filepath.Join(root, "-home-you-b"), 0o755)
	if got := All(); len(got) != 2 {
		t.Errorf("All = %v, want the project created since as well", got)
	}
}

// Unrelated projects have no order of preference: the newest session wins,
// and one written before launch never does.
func TestNewestSinceTakesTheNewestInAnyDir(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	os.MkdirAll(a, 0o755)
	os.MkdirAll(b, 0o755)
	older, newer := filepath.Join(a, "older.jsonl"), filepath.Join(b, "newer.jsonl")
	os.WriteFile(older, []byte("{}\n"), 0o644)
	os.WriteFile(newer, []byte("{}\n"), 0o644)
	os.Chtimes(older, nowMinus(10), nowMinus(10))
	os.Chtimes(newer, nowMinus(5), nowMinus(5))

	if dir, p := NewestSince([]string{a, b}, nowMinus(60)); dir != b || p != newer {
		t.Errorf("got %s %s, want the newer session in b", dir, p)
	}
	if _, p := NewestSince([]string{a, b}, nowMinus(1)); p != "" {
		t.Errorf("picked %s, written before launch", p)
	}
}

func nowMinus(sec int) time.Time { return time.Now().Add(-time.Duration(sec) * time.Second) }
