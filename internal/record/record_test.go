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
