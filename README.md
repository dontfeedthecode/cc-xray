# ccxray

Watch a Claude Code turn as it happens, in a terminal tab beside it.

Claude Code shows you a scrolling conversation. `ccxray` shows you the
*shape* of the turn: which model and effort level are actually in force, what
each action cost in tokens and time, and — the part that is otherwise
invisible — the moment a skill changes the model or effort out from under you.

```
▎ Run the test suite and fix whatever fails.

  MODEL             ACTION                                           OUT      Δt
  ────────────────────────────────────────────────────────────────────────────
  opus-5 (high)     Run the full test suite                          103    2.0s
× opus-5 (high)     Re-run the failing package with -race             12    1.0s
  opus-5 (high)     Skill  audit-deps                                 84    1.4s
  ╰─▶ ╭─ forked → sonnet-5 (medium)  ·  audit-deps  ·  agent a8aa6be7
      │✎ Check every module against the advisory database            410   20.3s
      │  Read  go.sum                                                747    8.5s
      ╰─ returned  9 req  ·  3,437 out  ·  49k ctx  ·  1m 11s
✎ opus-5 (high)     answer  Two modules are behind, the test fail… 1,512       —
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

Or from inside Claude Code, prefix it with `!` to run it in place:
`! curl -fsSL https://raw.githubusercontent.com/dontfeedthecode/ccxray/main/install.sh | sh`

It downloads a prebuilt binary for your platform, checks it against the
release's checksums, and puts it in `~/.local/bin` — nothing to build, no
`sudo`. Set `CCXRAY_INSTALL_DIR` to put it elsewhere, or `CCXRAY_VERSION=vX.Y.Z`
to pin a release. To uninstall, delete the binary.

With Go instead: `go install github.com/dontfeedthecode/ccxray/cmd/ccxray@latest`.
Or grab a binary from [Releases](https://github.com/dontfeedthecode/ccxray/releases).

## Use

Open a second terminal tab **in the directory Claude Code is working in**, and run:

```sh
ccxray
```

It starts empty and attaches to the first session Claude Code writes after
launch — start ccxray, then send your prompt — and keeps up when you `/clear`.
Earlier runs are never replayed; use `--session <id>` for one of those.
Running from a subdirectory is fine — it watches the parents up to the project.

| | |
|---|---|
| `↑` `↓` `k` `j` | scroll a line |
| `pgup` `pgdn` | scroll a page |
| `g` / `G` | jump to the top / follow the live edge |
| `c` | clear the panel and wait for the next session to write |
| `q` | quit |

The panel follows new rows like `tail -f`; scrolling up pauses that so
incoming rows don't yank the view away, and `G` resumes.

```
ccxray --project <dir>     # a directory other than the current one
ccxray --session <id>      # pin to one existing session instead of waiting
ccxray --ascii             # plain glyphs for terminals that widen box-drawing
```

## What it shows

**Model and effort on every row.** Usually unchanging — which is the point.
When something does change it, a band names both values.

**Forked skills, nested** under the call that launched them, on the model and
effort the fork actually ran on.

**When forks overlapped.** Two or more forks in a turn get a lane each below
the table on one time axis, with the peak that ran at once. Nothing in the
transcript says forks ran in parallel; overlapping spans are the only
evidence, so that is what the lanes measure.

**Δt per action**, so a 20-second step is obvious. **Failures** `×`,
**thinking** `✎`.

A turn ends when you type the next prompt and nothing else — a background
agent reporting back mid-turn does not start a new one.

## Requirements

- [Claude Code](https://claude.com/claude-code) (validated against `2.1.278`)
- macOS or Linux, `amd64` or `arm64`

The transcript format is internal to Claude Code and changes between
versions, so `ccxray` parses defensively: unknown fields are ignored, a bad
line is skipped rather than fatal, and a version mismatch shows a notice.

## How it works

Claude Code writes every session to
`~/.claude/projects/<slugged-cwd>/<session-id>.jsonl`, appending as the turn
runs. `ccxray` tails that file, tracking a byte offset so it only parses what
is new, and rebuilds the turn from it.

Forked skills live in a `subagents/` directory beside the session file, named
by an `agentId` the parent transcript carries, so the correlation is exact.
A fork is announced two different ways — `toolUseResult.status == "forked"`
when a `Skill` call launched it, and a `<forked-skill-launch>` payload on a
`system`/`local_command` record when a slash command did.

Counting tokens correctly turns out to be the fiddly part:

1. **Dedupe by `requestId`.** One assistant message is written as several
   records, and `output_tokens` is repeated in full on every one. Summing per
   record roughly doubles the total.
2. **`cache_read_input_tokens` is a peak, not a sum.** Every request re-reads
   the whole cache.
3. **A segment's duration is last record − first record** within it, never the
   gap to the next segment, which would swallow time you spent idle.
4. **`system` / `turn_duration`** is authoritative for the turn total.

Each produced plausible, wrong numbers before it was fixed. The test suite
asserts the exact figures of a known turn to keep them fixed.

Deciding what counts as a prompt is the other fiddly part. Claude Code writes
plenty into `user` records that nobody typed: skill bodies, command echoes,
attachment notes, background-agent notifications, and the summary handed
across a `/compact`. A prompt with an image attached is a block array rather
than a bare string, so both shapes are read. A slash command is a real prompt
when work follows it and noise when it does not, so it opens a turn only once
the model acts on it.

A `/compact` shows up as the first row of the next turn, with the context it
dropped — the reason the footer's context figure just collapsed.

## Trying it out

A real turn is whatever you happen to be doing, so the repo ships a skill
that drives a deliberately varied one. Open two tabs in this directory: in the
first `go run ./cmd/ccxray`, in the second start Claude Code and run
`/ccxray-demo`. It works through plain shell calls, several named tools, a
deliberate failure, and two skills that move the model and effort out from
under the turn.

Those skills live in `.claude/skills/` here rather than `~/.claude/skills`, so
they arrive with a clone. Claude Code loads them from the working directory at
startup — start it **from this directory**, and restart after a pull.

Two frontmatter keys matter, and they do not behave the same way:

| key | effect |
| --- | --- |
| `model:` | inert on its own; applies only with `context: fork` |
| `effort:` | applies without forking, but only if the skill is resolved before the turn's first request |

That second condition is why a slash command always works and a `Skill` call
only does when the model reaches for it immediately: one preceding request is
enough to lose the override silently.

`ccxray-demo-probe` sets both and forks; `ccxray-demo-effort` sets only
`effort:` and does not fork. Watch the MODEL column across both.

## Development

```sh
go test ./...
go run ./cmd/ccxray
```

`internal/turn/testdata/session.jsonl` is **synthetic**: the token arithmetic
and record structure come from a real session, but every prompt, reply, path
and identifier is fabricated. `internal/ui/testdata/fork-agent.jsonl` is a
real forked-skill transcript with its paths, tool output and system context
replaced — a fork's records differ in shape from a main session's.

Releases are cut by pushing a `v*` tag; [GoReleaser](https://goreleaser.com)
builds the binaries and checksums that `install.sh` downloads.

## Licence

MIT — see [LICENSE](LICENSE).
