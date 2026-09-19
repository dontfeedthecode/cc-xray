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
