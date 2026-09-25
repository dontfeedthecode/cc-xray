package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	fm := Parse("---\nname: lighthouse\ndescription: \"audits: CWV\"\neffort: low\nallowed-tools:\n  - Bash\n---\n\nbody\neffort: max\n")
	if fm.Effort != "low" || !fm.AllowedTools || fm.Fork || fm.Model != "" {
		t.Errorf("parsed %+v", fm)
	}
	if Parse("no frontmatter\neffort: low\n").Effort != "" {
		t.Error("read effort from a file with no frontmatter")
	}
}

// Effort a model-invoked skill asked for and did not get is named, with the
// invocation that applies it reliably.
func TestCheckNamesTheReliableInvocation(t *testing.T) {
	w := Check("lighthouse", Frontmatter{Effort: "low", AllowedTools: true}, "medium", "claude-opus-5-5")
	if !strings.Contains(w, "effort: low ignored, ran at medium") ||
		!strings.Contains(w, "/lighthouse applies it reliably") {
		t.Errorf("warning = %q", w)
	}
}

func TestCheckQuietWhenHonoured(t *testing.T) {
	for _, c := range []struct {
		fm            Frontmatter
		effort, model string
	}{
		{Frontmatter{Effort: "low", AllowedTools: true}, "low", "claude-opus-5-5"},
		{Frontmatter{Effort: "low"}, "", ""},                                  // nothing ran yet
		{Frontmatter{Effort: "low", Fork: true}, "medium", "claude-opus-5-5"}, // runs elsewhere
		{Frontmatter{Model: "opus"}, "medium", "claude-opus-5-5[1m]"},
		{Frontmatter{Model: "claude-opus-5-5"}, "medium", "claude-opus-5-5"},
	} {
		if w := Check("s", c.fm, c.effort, c.model); w != "" {
			t.Errorf("%+v ran %s/%s: unexpected warning %q", c.fm, c.effort, c.model, w)
		}
	}
}

// A skill's model applies in-thread; when it does not, the documented
// causes are auto mode and an allowlist, not a missing fork.
func TestCheckModelNotUsed(t *testing.T) {
	if w := Check("s", Frontmatter{Model: "sonnet"}, "high", "claude-opus-5"); !strings.Contains(w, "model: sonnet not used") {
		t.Errorf("warning = %q", w)
	}
}

// Edits to SKILL.md are picked up; an unreadable skill is not warned about.
func TestReaderRereadsOnChange(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "SKILL.md")
	var r Reader
	if _, ok := r.Read(d); ok {
		t.Fatal("read a skill that does not exist")
	}
	os.WriteFile(p, []byte("---\neffort: low\n---\n"), 0o644)
	if fm, _ := r.Read(d); fm.Effort != "low" {
		t.Fatalf("effort = %q", fm.Effort)
	}
	os.WriteFile(p, []byte("---\neffort: max\n---\n"), 0o644)
	later := mustStat(t, p).ModTime().Add(1e9)
	os.Chtimes(p, later, later)
	if fm, _ := r.Read(d); fm.Effort != "max" {
		t.Errorf("edit not picked up, effort = %q", fm.Effort)
	}
}

func mustStat(t *testing.T, p string) os.FileInfo {
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi
}
