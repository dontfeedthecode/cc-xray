package turn

import (
	"encoding/json"

	"github.com/dontfeedthecode/cc-xray/internal/record"
	"strings"
	"testing"
)

// The record order Claude Code writes when the user keeps talking while a
// background fork runs: /name launches the fork, a prompt arrives before it
// finishes, and the notice comes after that prompt's answer. Every identifier
// and every line of prose here is invented.
const backgroundSequence = `
{"type":"user","uuid":"u1","timestamp":"2026-09-23T14:53:34.338Z","message":{"role":"user","content":"<command-message>audit</command-message>\n<command-name>/audit</command-name>"}}
{"type":"system","subtype":"local_command","uuid":"s1","timestamp":"2026-09-23T14:53:34.353Z","content":"<local-command-stdout>Running in the background as @audit</local-command-stdout>\n<forked-skill-launch>{\"agentId\":\"agentbg1234\",\"skillName\":\"audit\"}</forked-skill-launch>"}
{"type":"user","uuid":"u2","timestamp":"2026-09-23T14:53:47.868Z","message":{"role":"user","content":"how's the weather?"}}
{"type":"assistant","uuid":"a1","requestId":"r1","timestamp":"2026-09-23T14:53:50.608Z","effort":"high","message":{"model":"claude-opus-5","stop_reason":"end_turn","content":[{"type":"text","text":"I can't see the weather from here."}],"usage":{"output_tokens":30}}}
{"type":"system","subtype":"turn_duration","uuid":"s2","timestamp":"2026-09-23T14:53:51.651Z","durationMs":3700}
{"type":"user","uuid":"u3","timestamp":"2026-09-23T14:54:03.901Z","message":{"role":"user","content":"<task-notification>\n<task-id>agentbg1234</task-id>\n<status>completed</status>\n</task-notification>"}}
{"type":"assistant","uuid":"a2","requestId":"r2","timestamp":"2026-09-23T14:54:09.386Z","effort":"high","message":{"model":"claude-opus-5","stop_reason":"end_turn","content":[{"type":"text","text":"The audit has finished."}],"usage":{"output_tokens":50}}}
{"type":"system","subtype":"turn_duration","uuid":"s3","timestamp":"2026-09-23T14:54:11.061Z","durationMs":7100}
`

func kinds(rows []Row) []string {
	var out []string
	for _, r := range rows {
		switch {
		case r.Fork != nil:
			out = append(out, "fork")
		case r.Change != nil && r.Change.Kind != "":
			out = append(out, r.Change.Kind)
		case r.Change != nil:
			out = append(out, "change")
		case r.Action != nil:
			out = append(out, "action:"+r.Action.Say)
		}
	}
	return out
}

// A prompt sent while a background fork runs used to open a new turn, which
// threw the fork away: its rows stopped updating and its return landed on a
// turn that knew nothing of it, so the panel never showed it finishing.
func TestPromptDuringBackgroundForkJoinsTheTurn(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(backgroundSequence), "\n")

	// after the prompt and its answer, before the fork reports back
	b := replay(t, strings.Join(lines[:5], "\n"))
	tn := b.Turn()
	if tn.Prompt != "/audit" {
		t.Errorf("Prompt = %q, want the turn that launched the fork", tn.Prompt)
	}
	if len(b.Forks()) != 1 {
		t.Fatalf("fork dropped by the follow-up prompt: %d forks", len(b.Forks()))
	}
	if tn.Complete {
		t.Error("turn marked complete while its background fork is still out")
	}
	want := []string{"fork", "prompt", "action:I can't see the weather from here."}
	if got := kinds(tn.Rows); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("rows = %v, want %v", got, want)
	}

	// the fork reports back
	b = replay(t, backgroundSequence)
	tn = b.Turn()
	if !b.Forks()[0].Notified {
		t.Error("notice did not mark the fork returned")
	}
	if !tn.Complete {
		t.Error("turn not complete once the fork returned and the answer ended")
	}
	want = append(want, "return", "action:The audit has finished.")
	if got := kinds(tn.Rows); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("rows = %v, want %v", got, want)
	}
	// timed by the wall clock, not by the last exchange's turn_duration
	if tn.Duration.Seconds() < 30 {
		t.Errorf("Duration = %v, want the whole span from the launch", tn.Duration)
	}

	// with nothing left out, the next prompt opens a fresh turn as usual
	b.Add(recordOf(t, `{"type":"user","uuid":"u9","timestamp":"2026-09-23T14:56:00Z","message":{"role":"user","content":"thanks"}}`))
	if b.Turn().Prompt != "thanks" || len(b.Forks()) != 0 {
		t.Errorf("next prompt did not start a fresh turn: %q, %d forks", b.Turn().Prompt, len(b.Forks()))
	}
}

// A fork whose transcript has ended counts as back even if the notice never
// comes, so it cannot hold every later prompt in its turn.
func TestFinishedForkReleasesTheTurn(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(backgroundSequence), "\n")
	b := replay(t, strings.Join(lines[:2], "\n"))
	b.Forks()[0].Nested = &Turn{Complete: true}
	b.Add(recordOf(t, lines[2]))
	if got := b.Turn().Prompt; got != "how's the weather?" {
		t.Errorf("Prompt = %q, want a fresh turn", got)
	}
}

func recordOf(t *testing.T, raw string) record.Record {
	t.Helper()
	var r record.Record
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	return r
}
