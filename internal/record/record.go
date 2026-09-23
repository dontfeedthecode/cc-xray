// Package record models the Claude Code session transcript (JSONL).
//
// The format is internal to Claude Code and changes between versions, so every
// field here is optional and nothing panics on absence. Validated against 2.1.278.
package record

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// ValidatedVersion is the Claude Code version this parser was checked against.
const ValidatedVersion = "2.1.278"

// SyntheticModel marks locally-generated messages that never hit the API.
// They carry no effort and no usage and must be skipped entirely.
const SyntheticModel = "<synthetic>"

type Usage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	Details             struct {
		ThinkingTokens int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

type Block struct {
	Type      string          `json:"type"` // thinking | text | tool_use | tool_result
	Name      string          `json:"name"` // tool name, when Type == tool_use
	ID        string          `json:"id"`   // tool_use id, paired with ToolUseID
	ToolUseID string          `json:"tool_use_id"`
	Text      string          `json:"text"`
	IsError   bool            `json:"is_error"`
	Input     json.RawMessage `json:"input"`
	Content   json.RawMessage `json:"content"`
}

// Failed reports whether a tool_result represents an error. Claude Code marks
// these either with is_error or an inline <tool_use_error> marker.
func (b Block) Failed() bool {
	if b.Type != "tool_result" {
		return false
	}
	if b.IsError {
		return true
	}
	return bytes.Contains(b.Content, []byte("tool_use_error"))
}

type Message struct {
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      Usage  `json:"usage"`
	// Content is an array for assistant messages, and for user prompts is
	// either a bare string or an array when something is attached, so it is
	// decoded lazily by Blocks/Text/PromptText.
	Content json.RawMessage `json:"content"`
}

// ToolUseResult is a sidecar on the user record that carries a tool's result.
// For a skill with `context: fork` it identifies the subagent that ran it.
type ToolUseResult struct {
	Success     bool   `json:"success"`
	CommandName string `json:"commandName"`
	Status      string `json:"status"` // "forked" when the skill ran as a subagent
	Background  bool   `json:"background"`
	AgentID     string `json:"agentId"` // -> subagents/agent-<AgentID>.jsonl
}

// CompactMetadata describes a /compact: the context it replaced and the one
// it produced. Carried on system/compact_boundary.
type CompactMetadata struct {
	Trigger    string `json:"trigger"` // "manual" | "auto"
	PreTokens  int    `json:"preTokens"`
	PostTokens int    `json:"postTokens"`
	DurationMs int    `json:"durationMs"`
}

type Record struct {
	Type             string  `json:"type"`
	Subtype          string  `json:"subtype"`
	UUID             string  `json:"uuid"`
	RequestID        string  `json:"requestId"`
	APIBlockIndex    int     `json:"apiBlockIndex"`
	Timestamp        string  `json:"timestamp"`
	Message          Message `json:"message"`
	Effort           string  `json:"effort"`
	PerTurnEffort    *string `json:"perTurnEffort"`
	AttributionSkill string  `json:"attributionSkill"`
	IsSidechain      bool    `json:"isSidechain"`
	IsMeta           bool    `json:"isMeta"`
	IsCompactSummary bool    `json:"isCompactSummary"`
	SessionID        string  `json:"sessionId"`
	Version          string  `json:"version"`
	CWD              string  `json:"cwd"`
	GitBranch        string  `json:"gitBranch"`

	// type == "system", subtype == "turn_duration"
	DurationMs   int `json:"durationMs"`
	MessageCount int `json:"messageCount"`

	// type == "system", subtype == "compact_boundary"
	CompactMetadata *CompactMetadata `json:"compactMetadata"`

	// Decoded lazily: this field is an object on most records but a bare
	// string on some. A typed field makes json reject the entire record.
	ToolUseResult json.RawMessage `json:"toolUseResult"`

	// Top-level content, distinct from Message.Content. A string on
	// system records and an object elsewhere, so it is decoded lazily for
	// the same reason.
	Content json.RawMessage `json:"content"`

	// sidecar record types
	LastPrompt     string `json:"lastPrompt"`
	AITitle        string `json:"aiTitle"`
	PermissionMode string `json:"permissionMode"`
}

// Blocks decodes assistant content, returning nil when it is not an array.
func (m Message) Blocks() []Block {
	if len(m.Content) == 0 || m.Content[0] != '[' {
		return nil
	}
	var b []Block
	if err := json.Unmarshal(m.Content, &b); err != nil {
		return nil
	}
	return b
}

// Text decodes user content, returning "" when it is not a bare string.
func (m Message) Text() string {
	if len(m.Content) == 0 || m.Content[0] != '"' {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err != nil {
		return ""
	}
	return s
}

func (r Record) Time() time.Time {
	t, err := time.Parse(time.RFC3339Nano, r.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Synthetic reports whether this record was generated locally. Rule 5: these
// must never reach the aggregator or every interruption splits a segment.
func (r Record) Synthetic() bool { return r.Message.Model == SyntheticModel }

// EffortLevel applies rule 7 — perTurnEffort overrides only when non-null.
func (r Record) EffortLevel() string {
	if r.PerTurnEffort != nil && *r.PerTurnEffort != "" {
		return *r.PerTurnEffort
	}
	return r.Effort
}

// ShortModel trims the vendor prefix: claude-opus-5 -> opus-5.
func ShortModel(m string) string {
	if m == "" || m == SyntheticModel {
		return m
	}
	return strings.TrimPrefix(m, "claude-")
}

// Forked reports whether this record announces a forked skill, and names the
// subagent that ran it. The linkage is exact: no timing heuristics needed.
func (r Record) Forked() (agentID, skill string, ok bool) {
	t, ok2 := r.Result()
	if !ok2 || t.Status != "forked" || t.AgentID == "" {
		return "", "", false
	}
	return t.AgentID, t.CommandName, true
}

// ForkParent returns the id of the Skill tool_use this fork answers. The
// announcement is only written when the subagent finishes, so its timestamp
// can be a minute later than the call that launched it and says nothing about
// where the fork belongs on screen; this id does.
func (r Record) ForkParent() string {
	for _, b := range r.Message.Blocks() {
		if b.ToolUseID != "" {
			return b.ToolUseID
		}
	}
	return ""
}

// ForkedLaunch reports a fork that a slash command started. A skill invoked
// as /name is launched by Claude Code itself rather than by a Skill tool
// call, so it has no toolUseResult to carry the linkage: the announcement
// arrives on a system/local_command record as a <forked-skill-launch>
// payload. Reading only the tool-call form left these forks invisible.
func (r Record) ForkedLaunch() (agentID, skill string, ok bool) {
	if r.Type != "system" || r.Subtype != "local_command" {
		return "", "", false
	}
	var body string
	if len(r.Content) == 0 || r.Content[0] != '"' ||
		json.Unmarshal(r.Content, &body) != nil {
		return "", "", false
	}
	const open, close = "<forked-skill-launch>", "</forked-skill-launch>"
	i := strings.Index(body, open)
	if i < 0 {
		return "", "", false
	}
	rest := body[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", "", false
	}
	var v struct {
		AgentID   string `json:"agentId"`
		SkillName string `json:"skillName"`
	}
	if json.Unmarshal([]byte(rest[:j]), &v) != nil || v.AgentID == "" {
		return "", "", false
	}
	return v.AgentID, v.SkillName, true
}

// Result decodes toolUseResult when it is an object, reporting false when it
// is a bare string or absent.
func (r Record) Result() (ToolUseResult, bool) {
	var t ToolUseResult
	if len(r.ToolUseResult) == 0 || r.ToolUseResult[0] != '{' {
		return t, false
	}
	if json.Unmarshal(r.ToolUseResult, &t) != nil {
		return t, false
	}
	return t, true
}

// commandWrappers are local command echoes, not real user prompts (rule 8).
var commandWrappers = []string{
	"<local-command-caveat>", "<local-command-stdout>",
	"<command-name>", "<command-message>", "<command-args>",
}

// harnessBlocks open a record that Claude Code injected itself, not something
// the user typed. A background agent reporting back arrives as an ordinary
// user record — no isMeta, plain string content — so it satisfied every other
// prompt test and started a fresh turn in the middle of the work it was
// reporting on, discarding the fork rows already gathered. Matched as a
// prefix rather than a substring on purpose: a user quoting a notification
// back (which is exactly how this bug got reported) is still a real prompt.
var harnessBlocks = []string{"<task-notification>", "<system-reminder>"}

// SlashCommand returns the command a user typed, for a record that is only a
// slash-command echo. Rule 8 rejects these as prompts because most are local
// UI (/clear, /config) that start no work at all — but a skill invoked as
// /name opens a real turn, and treating it as an echo meant the whole run was
// invisible. The two are indistinguishable here, so the caller decides by
// whether assistant work follows.
func (r Record) SlashCommand() string {
	if r.Type != "user" || r.IsMeta {
		return ""
	}
	t := r.Message.Text()
	if i := strings.Index(t, "<command-name>"); i >= 0 {
		rest := t[i+len("<command-name>"):]
		j := strings.Index(rest, "</command-name>")
		if j < 0 {
			return ""
		}
		return strings.TrimSpace(rest[:j])
	}
	return bareCommand(t)
}

// bareCommand recognises a slash command in the raw form the user typed it.
// Claude Code writes this alongside the wrapped echo for some commands and
// instead of it for others, as an ordinary user record — no isMeta, no
// wrapper, plain string content — so it satisfied every prompt test. The cost
// showed on /compact, which starts no work of its own: it opened a turn that
// never got a request, and the panel sat spinning on it until the next real
// prompt. Routed through the same path as the echo, it opens a turn only if
// work follows, which is the existing rule.
//
// The whole line is returned rather than just the name, because a bare record
// carries the arguments inline where the wrapped form puts them in their own
// tag. A path is not a command: anything further in the first token — /tmp/x,
// /usr/bin, ./go — rules it out.
func bareCommand(t string) string {
	t = strings.TrimSpace(t)
	f := strings.Fields(t)
	if len(f) == 0 || len(f[0]) < 2 || f[0][0] != '/' ||
		strings.ContainsAny(f[0][1:], "/.") {
		return ""
	}
	return t
}

// PromptText returns the words the user actually typed. Content is a bare
// string for a plain prompt but a block array whenever something is attached
// (an image, a pasted file), so both shapes have to be read: keying only off
// the string form made every prompt with a screenshot invisible, and the
// panel went on showing the previous turn. A tool_result block means the
// record is carrying results back to the model, not opening a turn.
func (m Message) PromptText() string {
	if s := m.Text(); s != "" {
		return s
	}
	var parts []string
	for _, b := range m.Blocks() {
		if b.Type == "tool_result" || b.ToolUseID != "" {
			return ""
		}
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// IsUserPrompt reports whether this record starts a turn (rule 8).
func (r Record) IsUserPrompt() bool {
	if r.Type != "user" {
		return false
	}
	// isMeta marks text Claude Code injects on the user's behalf: skill
	// bodies, attachment notes, local command echoes. isCompactSummary marks
	// the summary handed across a /compact — treating that as a prompt made
	// a freshly opened panel pin the summary instead of the real turn.
	if r.IsMeta || r.IsCompactSummary {
		return false
	}
	t := strings.TrimSpace(r.Message.PromptText())
	if t == "" {
		return false
	}
	for _, w := range commandWrappers {
		if strings.Contains(t, w) {
			return false
		}
	}
	for _, w := range harnessBlocks {
		if strings.HasPrefix(t, w) {
			return false
		}
	}
	return true
}
