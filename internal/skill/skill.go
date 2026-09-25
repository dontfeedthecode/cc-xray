// Package skill reads what a skill's frontmatter asks for, so the panel can
// say when Claude Code did not honour it.
package skill

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dontfeedthecode/cc-xray/internal/usage"
)

// Frontmatter is the part of SKILL.md that decides how a skill runs.
type Frontmatter struct {
	Effort       string
	Model        string
	Fork         bool // context: fork
	AllowedTools bool // allowed-tools is declared
}

// Parse reads the frontmatter block at the top of a SKILL.md. Only flat
// "key: value" lines are understood, which covers every key used here;
// anything it cannot read is left empty rather than guessed.
func Parse(text string) Frontmatter {
	var f Frontmatter
	sc := bufio.NewScanner(strings.NewReader(text))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return f
	}
	for sc.Scan() {
		ln := sc.Text()
		if strings.TrimSpace(ln) == "---" {
			break
		}
		k, v, ok := strings.Cut(ln, ":")
		if !ok || strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t") {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "effort":
			f.Effort = v
		case "model":
			f.Model = v
		case "context":
			f.Fork = v == "fork"
		case "allowed-tools":
			// A block list puts its items on the lines below, so the key
			// alone is enough to count as declared.
			f.AllowedTools = true
		}
	}
	return f
}

// Reader caches frontmatter by file, re-reading only when the file changes.
type Reader struct {
	cache map[string]cached
}

type cached struct {
	mod time.Time
	fm  Frontmatter
	ok  bool
}

// Read returns the frontmatter of the skill in dir. It reports false when
// the file cannot be read, so the caller stays quiet rather than warn on
// nothing.
func (r *Reader) Read(dir string) (Frontmatter, bool) {
	p := filepath.Join(dir, "SKILL.md")
	fi, err := os.Stat(p)
	if err != nil {
		return Frontmatter{}, false
	}
	if c, hit := r.cache[p]; hit && c.mod.Equal(fi.ModTime()) {
		return c.fm, c.ok
	}
	b, err := os.ReadFile(p)
	c := cached{mod: fi.ModTime(), ok: err == nil}
	if err == nil {
		c.fm = Parse(string(b))
	}
	if r.cache == nil {
		r.cache = map[string]cached{}
	}
	r.cache[p] = c
	return c.fm, c.ok
}

// Check compares what the skill called name asked for with what its first request ran at,
// and describes any gap. It returns "" when everything was honoured or
// there is nothing to compare yet.
//
// A forked skill runs in its own transcript, so what the parent ran at says
// nothing about it and it is never checked here.
func Check(name string, fm Frontmatter, ranEffort, ranModel string) string {
	if fm.Fork || (ranEffort == "" && ranModel == "") {
		return ""
	}
	var out []string
	if fm.Effort != "" && ranEffort != "" && fm.Effort != ranEffort {
		// Not documented. When the model calls a skill, Claude Code applies
		// its effort only some of the time: identical runs, down to the
		// transcript, went either way, with or without allowed-tools. A
		// skill typed as /name is resolved before the turn's first request
		// and has taken effect every time.
		msg := "effort: " + fm.Effort + " ignored, ran at " + ranEffort +
			" · /" + name + " applies it reliably"
		out = append(out, msg)
	}
	if fm.Model != "" && ranModel != "" && !sameModel(fm.Model, ranModel) {
		// Documented: an in-thread model override is skipped when auto mode
		// does not support the model, or the organisation's availableModels
		// allowlist excludes it.
		out = append(out, "model: "+fm.Model+" not used · auto mode or an allowlist can block it")
	}
	return strings.Join(out, "   ")
}

// sameModel accepts an alias ("sonnet") or a full id for the model that ran.
func sameModel(asked, ran string) bool {
	asked, ran = usage.Canonical(asked), usage.Canonical(ran)
	if asked == ran || asked == "inherit" {
		return true
	}
	return !strings.HasPrefix(asked, "claude-") && strings.Contains(ran, "-"+asked)
}
