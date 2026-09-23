package record

import (
	"encoding/json"
	"testing"
)

// The bug that produced "skipped malformed lines": toolUseResult is an object
// on most records and a bare string on some. A typed field made encoding/json
// reject the ENTIRE record, silently losing its actions.
func TestToolUseResultBothShapes(t *testing.T) {
	object := `{"type":"user","uuid":"u1","toolUseResult":{"status":"forked","agentId":"a1","commandName":"lighthouse-audit"}}`
	str := `{"type":"user","uuid":"u2","toolUseResult":"Launching skill: lighthouse-audit"}`

	var r1 Record
	if err := json.Unmarshal([]byte(object), &r1); err != nil {
		t.Fatalf("object form rejected: %v", err)
	}
	if id, skill, ok := r1.Forked(); !ok || id != "a1" || skill != "lighthouse-audit" {
		t.Errorf("Forked() = %q %q %v", id, skill, ok)
	}

	var r2 Record
	if err := json.Unmarshal([]byte(str), &r2); err != nil {
		t.Fatalf("string form rejected — this is the whole bug: %v", err)
	}
	if _, ok := r2.Result(); ok {
		t.Error("a string toolUseResult should not decode as an object")
	}
	if _, _, ok := r2.Forked(); ok {
		t.Error("string form must not be treated as a fork")
	}
}

func TestToolResultErrorDetection(t *testing.T) {
	var r Record
	raw := `{"type":"user","message":{"content":[
	  {"type":"tool_result","tool_use_id":"t1","content":"<tool_use_error>boom</tool_use_error>"},
	  {"type":"tool_result","tool_use_id":"t2","content":"fine"},
	  {"type":"tool_result","tool_use_id":"t3","is_error":true,"content":"x"}]}}`
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"t1": true, "t2": false, "t3": true}
	for _, b := range r.Message.Blocks() {
		if got := b.Failed(); got != want[b.ToolUseID] {
			t.Errorf("%s Failed() = %v, want %v", b.ToolUseID, got, want[b.ToolUseID])
		}
	}
}

// A compact summary and an injected skill body are user records with plain
// text content, and both were opening turns of their own. Only what the
// person actually typed may do that.
func TestIsUserPromptRejectsInjectedText(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"plain prompt", `{"type":"user","message":{"role":"user","content":"run the tests"}}`, true},
		{"compact summary", `{"type":"user","isCompactSummary":true,"isVisibleInTranscriptOnly":true,
			"message":{"role":"user","content":"This session is being continued from a previous conversation."}}`, false},
		{"injected skill body", `{"type":"user","isMeta":true,
			"message":{"role":"user","content":[{"type":"text","text":"Base directory for this skill: /skills/x"}]}}`, false},
		{"local command echo", `{"type":"user","message":{"role":"user","content":"<command-name>/compact</command-name>"}}`, false},
		{"tool result carrier", `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`, false},
		{"assistant record", `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var r Record
			if err := json.Unmarshal([]byte(c.line), &r); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := r.IsUserPrompt(); got != c.want {
				t.Errorf("IsUserPrompt() = %v, want %v", got, c.want)
			}
		})
	}
}

// Attaching a screenshot turns message.content from a string into a block
// array. Reading only the string form made those prompts invisible, so the
// panel kept showing the previous turn.
func TestPromptTextReadsAttachedPrompt(t *testing.T) {
	const line = `{"type":"user","message":{"role":"user","content":[
		{"type":"text","text":"here is a screenshot, what is wrong?"},
		{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}}`
	var r Record
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !r.IsUserPrompt() {
		t.Fatal("prompt with an attachment must start a turn")
	}
	if got, want := r.Message.PromptText(), "here is a screenshot, what is wrong?"; got != want {
		t.Errorf("PromptText() = %q, want %q", got, want)
	}
}

// A turn ends when the user speaks, and nothing else. A background agent
// reporting back arrives as an ordinary user record with no isMeta flag and
// plain string content, so it passed every other prompt test and started a
// fresh turn in the middle of the work it was reporting on.
func TestIsUserPromptIgnoresHarnessInjectedBlocks(t *testing.T) {
	user := func(text string) Record {
		b, err := json.Marshal(text)
		if err != nil {
			t.Fatal(err)
		}
		return Record{Type: "user", Message: Message{Content: b}}
	}
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"real prompt", "audit the homepage please", true},
		{"task notification", "<task-notification>\n<task-id>abc</task-id>\n</task-notification>", false},
		{"system reminder", "<system-reminder>\nremember to do X\n</system-reminder>", false},
		// The bug was reported by pasting a notification into a message; that
		// is still the user speaking, so a substring match would be wrong.
		{"user quoting a notification", "one observation,\n\n<task-notification> finished", true},
		{"notification named in prose", "why did the <task-notification> reset it?", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := user(c.text).IsUserPrompt(); got != c.want {
				t.Errorf("IsUserPrompt(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

// A skill invoked as /name is launched by Claude Code, not by a Skill tool
// call, so it carries no toolUseResult. Its announcement arrives on a
// system/local_command record instead, and reading only the tool-call form
// left every slash-launched fork invisible.
func TestForkedLaunchFromSlashCommand(t *testing.T) {
	raw := `{"type":"system","subtype":"local_command",
	  "content":"<local-command-stdout>Running in the background as @lighthouse-audit</local-command-stdout>\n<forked-skill-launch>{\"agentId\":\"a282e7de257a12292\",\"skillName\":\"lighthouse-audit\",\"description\":\"/lighthouse-audit\"}</forked-skill-launch>"}`
	var r Record
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	id, skill, ok := r.ForkedLaunch()
	if !ok {
		t.Fatal("slash-launched fork not recognised")
	}
	if id != "a282e7de257a12292" || skill != "lighthouse-audit" {
		t.Errorf("got %q %q", id, skill)
	}

	for _, other := range []string{
		`{"type":"system","subtype":"local_command","content":"<local-command-stdout>ok</local-command-stdout>"}`,
		`{"type":"system","subtype":"compact_boundary","content":"x"}`,
		// content is an object on other record types and must not break parsing
		`{"type":"attachment","subtype":"local_command","content":{"type":"deferred_tools_delta"}}`,
	} {
		var o Record
		if err := json.Unmarshal([]byte(other), &o); err != nil {
			t.Fatalf("record rejected outright: %v  (%s)", err, other)
		}
		if _, _, ok := o.ForkedLaunch(); ok {
			t.Errorf("false positive on %s", other)
		}
	}
}
