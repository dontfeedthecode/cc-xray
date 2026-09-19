package turn

import (
	"encoding/json"
	"testing"
)

// Regression for "most actions are Bash which doesn't explain much": every
// tool that appears in a real session must produce a useful label.
func TestDescribeCoversRealTools(t *testing.T) {
	cases := []struct {
		name, input, want string
	}{
		{"Bash", `{"description":"Audit packaging readiness"}`, "Audit packaging readiness"},
		{"Bash", `{"command":"go test ./...\nmore"}`, "go test ./..."},
		{"Write", `{"file_path":"/Users/x/ccxray/internal/ui/render.go"}`, "render.go"},
		{"Read", `{"file_path":"/a/b/SKILL.md"}`, "SKILL.md"},
		{"Grep", `{"pattern":"attributionSkill","path":"/a/internal"}`, "attributionSkill  in internal"},
		{"Skill", `{"skill":"lighthouse-audit","args":"http://x"}`, "lighthouse-audit  http://x"},
		{"Agent", `{"subagent_type":"claude-code-guide"}`, "claude-code-guide"},
		{"ToolSearch", `{"query":"select:Read"}`, "select:Read"},
		{"ExitPlanMode", `{}`, "submitted the plan for approval"},
		{"AskUserQuestion", `{"questions":[{"header":"Licence"},{"header":"Distribution"}]}`, "Licence (+1)"},
		{"Artifact", `{"action":"publish","file_path":"/tmp/ccxray-log.html"}`, "publish  ccxray-log.html"},
		{"mcp__claude-in-chrome__navigate", `{"url":"http://localhost:8899"}`, "http://localhost:8899"},
	}
	for _, c := range cases {
		if got := describe(c.name, json.RawMessage(c.input)); got != c.want {
			t.Errorf("describe(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

// These three produced a bare tool name with no label before.
func TestPreviouslyUnlabelledToolsNowSpeak(t *testing.T) {
	for _, n := range []string{"AskUserQuestion", "ExitPlanMode", "Artifact"} {
		if got := describe(n, json.RawMessage(`{}`)); got == "" {
			t.Errorf("%s still renders with no description", n)
		}
	}
}
