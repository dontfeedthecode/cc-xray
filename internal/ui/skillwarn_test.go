package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// transcript calls a skill in dir whose first request runs at effort.
func skillTranscript(dir, effort string) string {
	return `{"type":"user","uuid":"u1","timestamp":"2026-01-02T10:00:00.000Z","message":{"role":"user","content":"audit the site"}}
{"type":"assistant","uuid":"a1","requestId":"r1","timestamp":"2026-01-02T10:00:02.000Z","effort":"medium","message":{"model":"claude-opus-5-5","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"Skill","input":{"skill":"lighthouse"}}]}}
{"type":"user","uuid":"u3","isMeta":true,"sourceToolUseID":"t1","timestamp":"2026-01-02T10:00:02.100Z","message":{"role":"user","content":[{"type":"text","text":"Base directory for this skill: ` + dir + `\n\nRun it."}]}}
{"type":"assistant","uuid":"a2","requestId":"r2","timestamp":"2026-01-02T10:00:05.000Z","effort":"` + effort + `","attributionSkill":"lighthouse","message":{"model":"claude-opus-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"Done."}]}}
`
}

func renderSkill(t *testing.T, frontmatter, ranEffort string) string {
	t.Helper()
	d := t.TempDir()
	skill := filepath.Join(d, "skills", "lighthouse")
	os.MkdirAll(skill, 0o755)
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: lighthouse\n"+frontmatter+"---\n\nbody\n"), 0o644)
	p := filepath.Join(d, "s.jsonl")
	os.WriteFile(p, []byte(skillTranscript(skill, ranEffort)), 0o644)

	m := NewModel(Options{Path: p})
	m.poll()
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = mm.(Model)
	m.refresh()
	return stripANSI(m.View())
}

func TestIgnoredSkillEffortIsFlagged(t *testing.T) {
	v := renderSkill(t, "effort: low\n", "medium")
	if !strings.Contains(v, "! effort: low ignored, ran at medium · /lighthouse applies it reliably") {
		t.Errorf("no warning for the ignored effort:\n%s", v)
	}
}

func TestHonouredSkillEffortIsQuiet(t *testing.T) {
	v := renderSkill(t, "effort: low\nallowed-tools: Bash\n", "low")
	if strings.Contains(v, "ignored") {
		t.Errorf("warned about an effort that took effect:\n%s", v)
	}
}
