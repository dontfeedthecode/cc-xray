package turn

import "testing"

// The record order Claude Code writes when the model calls a skill: the call,
// its result, then the skill's instructions naming its folder. Identifiers
// and paths are invented.
const skillCall = `
{"type":"user","uuid":"u1","timestamp":"2026-01-02T10:00:00.000Z","message":{"role":"user","content":"audit the site"}}
{"type":"assistant","uuid":"a1","requestId":"r1","timestamp":"2026-01-02T10:00:02.000Z","effort":"medium","message":{"model":"claude-opus-5-5","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t1","name":"Skill","input":{"skill":"lighthouse","args":"https://example.test/"}}]}}
{"type":"user","uuid":"u2","timestamp":"2026-01-02T10:00:02.100Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"Launching skill: lighthouse"}]},"toolUseResult":{"success":true,"commandName":"lighthouse"}}
{"type":"user","uuid":"u3","isMeta":true,"sourceToolUseID":"t1","timestamp":"2026-01-02T10:00:02.100Z","message":{"role":"user","content":[{"type":"text","text":"Base directory for this skill: /work/My Sites/.claude/skills/lighthouse\n\nRun the audit."}]}}
{"type":"assistant","uuid":"a2","requestId":"r2","timestamp":"2026-01-02T10:00:05.000Z","effort":"medium","attributionSkill":"lighthouse","message":{"model":"claude-opus-5-5","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"description":"run lighthouse"}}]}}
{"type":"user","uuid":"u4","timestamp":"2026-01-02T10:00:09.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"Exit code 1"}]}}
`

func TestSkillCallKnowsItsFolderAndWhatRan(t *testing.T) {
	b := replay(t, skillCall)
	a := b.Turn().Rows[0].Action
	if a == nil || a.SkillName != "lighthouse" {
		t.Fatalf("first row is not the Skill call: %+v", b.Turn().Rows[0])
	}
	if a.SkillDir != "/work/My Sites/.claude/skills/lighthouse" {
		t.Errorf("dir = %q", a.SkillDir)
	}
	if a.RanEffort != "medium" || a.RanModel != "claude-opus-5-5" {
		t.Errorf("ran %q/%q, want the skill's first request", a.RanEffort, a.RanModel)
	}
}

// A failed tool result must mark its row. User records used to return before
// reaching this, so no row was ever marked.
func TestFailedToolResultMarksItsRow(t *testing.T) {
	b := replay(t, skillCall)
	var bash *Action
	for _, r := range b.Turn().Rows {
		if r.Action != nil && r.Action.Tool == "Bash" {
			bash = r.Action
		}
	}
	if bash == nil || !bash.Failed {
		t.Errorf("Bash row not marked failed: %+v", bash)
	}
}

// A skill called again is not reloaded, so its folder comes from the first call.
func TestRecalledSkillKeepsItsFolder(t *testing.T) {
	b := replay(t, skillCall+`
{"type":"user","uuid":"u5","timestamp":"2026-01-02T10:05:00.000Z","message":{"role":"user","content":"again"}}
{"type":"assistant","uuid":"a3","requestId":"r3","timestamp":"2026-01-02T10:05:02.000Z","effort":"medium","message":{"model":"claude-opus-5-5","stop_reason":"tool_use","content":[{"type":"tool_use","id":"t3","name":"Skill","input":{"skill":"lighthouse"}}]}}
{"type":"user","uuid":"u6","isMeta":true,"sourceToolUseID":"t3","timestamp":"2026-01-02T10:05:02.100Z","message":{"role":"user","content":"Skill /lighthouse is already loaded above; instructions unchanged."}}
`)
	if a := b.Turn().Rows[0].Action; a == nil || a.SkillDir != "/work/My Sites/.claude/skills/lighthouse" {
		t.Errorf("second call lost the folder: %+v", a)
	}
}
