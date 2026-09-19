# ccxray

Watch a Claude Code turn as it happens, in a terminal tab beside it.

Claude Code shows you a scrolling conversation. `ccxray` shows you the
*shape* of the turn: which model and effort level are actually in force, what
each action cost in tokens and time, and — the part that is otherwise
invisible — the moment a skill changes the model out from under you.

```
▎ Run a lighthouse audit of http://localhost:8080/ and report any substantial
▎ issues.

  MODEL             ACTION                                           OUT      Δt
  ────────────────────────────────────────────────────────────────────────────
  opus-5 (high)     Skill  lighthouse-audit  http://localhost:808…   103    2.0s
  ╭─ forked → sonnet-5  ·  lighthouse-audit  ·  agent a8aa6be7
  │✎ sonnet-5 (high)   Run the Lighthouse desktop audit against t…   410   20.3s
  │  sonnet-5 (medium) Read  report.json                             747    8.5s
  ╰─ returned  9 req  ·  3,437 out  ·  49k ctx  ·  1m 11s
× opus-5 (high)     A command that failed, marked and tinted          12    1.0s
✎ opus-5 (high)     answer  I'll use the lighthouse-audit skill f… 1,512       —
  ────────────────────────────────────────────────────────────────────────────
  turn complete   15 req  ·  63k ctx                               6,080  1m 48s
```

Most actions are shell commands, so `Bash` is the assumed default and goes
unnamed — the description is the useful part and gets the space. Anything
else announces itself: `Read`, `Grep`, `Skill`, an MCP tool. `✎` marks a row
the model thought before, `×` marks a call that failed.

It is **read-only**. It tails the transcript Claude Code already writes and
never modifies anything.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/dontfeedthecode/ccxray/main/install.sh | sh
```

Or, with Go installed:

```sh
go install github.com/dontfeedthecode/ccxray/cmd/ccxray@latest
```

Or grab a binary from [Releases](https://github.com/dontfeedthecode/ccxray/releases).

## Use

Open a second terminal tab **in the directory Claude Code is working in**, and run:

```sh
ccxray
```

It finds the newest session for that directory on its own, and keeps up when
you `/clear` or start a new one. Running from a subdirectory is fine — it
walks up until it finds the project.

| | |
|---|---|
| `↑` `↓` `k` `j` | scroll a line |
| `pgup` `pgdn` | scroll a page |
| `g` / `G` | jump to the top / follow the live edge |
| `q` | quit |

The panel follows new rows like `tail -f`. Scroll up and it stops following
so incoming rows don't yank the view away; `G` resumes.

```
ccxray --project <dir>     # a directory other than the current one
ccxray --session <id>      # pin to one session instead of the newest
ccxray --ascii             # plain glyphs for terminals that widen box-drawing
```

## What it shows

**Model and effort on every row.** Usually unchanging — which is the point.
When something does change it, you see exactly where.

**Forked skills, nested.** A skill with `context: fork` in its frontmatter
runs as a subagent, on its own model, writing to its own transcript. `ccxray`
follows it and renders it inline under the call that launched it. This is the
only case where a skill's `model:` key actually takes effect; without
`context: fork` it is inert, and the panel will show you that too.

**Δt per action**, so a 20-second step is obvious.

**Failures**, marked `×`. **Thinking**, marked `✎`.

## Requirements

- [Claude Code](https://claude.com/claude-code) (validated against `2.1.277`)
- macOS or Linux, `amd64` or `arm64`

The transcript format is internal to Claude Code and changes between
versions. `ccxray` parses defensively — unknown fields are ignored, a bad line
is skipped rather than fatal — and shows a notice when the transcript version
differs from the one it was validated against.

## How it works

Claude Code writes every session to
`~/.claude/projects/<slugged-cwd>/<session-id>.jsonl`, appending as the turn
runs. `ccxray` tails that file, tracking a byte offset so it only parses what
is new, and rebuilds the turn from it.

Forked skills live in a `subagents/` directory beside the session file. The
parent transcript announces them with `toolUseResult.status == "forked"` and
an `agentId` that names the file, so the correlation is exact.

Counting tokens correctly turns out to be the fiddly part, and four rules
matter:

1. **Dedupe by `requestId`.** One assistant message is written as several
   records, and `output_tokens` is repeated in full on every one. Summing per
   record roughly doubles the total.
2. **`cache_read_input_tokens` is a peak, not a sum.** Every request re-reads
   the whole cache.
3. **A segment's duration is last record − first record** within it, never the
   gap to the next segment, which would swallow time you spent idle.
4. **`system` / `turn_duration`** is authoritative for the turn total.

Each of these produced plausible, wrong numbers before it was fixed. The test
suite asserts the exact figures of a known turn to keep them fixed.

Deciding what counts as a prompt is the other fiddly part. Claude Code writes
a lot of text into `user` records that nobody typed — injected skill bodies,
local command echoes, attachment notes, and the summary handed across a
`/compact`. Those carry `isMeta` or `isCompactSummary`, and `ccxray` skips
them. Meanwhile a prompt with an image attached is written as a block array
rather than a bare string, so both shapes are read.

A `/compact` shows up as the first row of the turn that follows it, with the
context it dropped — it is the reason the context figure in the footer just
collapsed.

## Trying it out

Watching a real turn is the point, but a real turn is whatever you happen to
be doing. The repo ships a skill that drives a deliberately varied one, so
you can see every kind of row at once.

Open two terminal tabs in this directory. In the first:

```sh
go run ./cmd/ccxray
```

In the second, start Claude Code and ask it to run the demo:

```
/ccxray-demo
```

It works through plain shell calls, several named tools, a deliberate
failure, and a skill that forks onto a different model — which is the one
thing you cannot see any other way. `.claude/skills/` holds both halves; the
`model:` line only takes effect because the probe declares `context: fork`.

## Development

```sh
go test ./...
go run ./cmd/ccxray
```

`internal/turn/testdata/session.jsonl` is a **synthetic** transcript: the
token arithmetic and record structure come from a real session, but every
prompt, reply, path and identifier is fabricated.

## Licence

MIT — see [LICENSE](LICENSE).
