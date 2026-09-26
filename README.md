# X-ray for Claude Code

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

Most actions are shell commands, so `Bash` (and `PowerShell` on Windows) is
the assumed default and goes unnamed — the description is the useful part and
gets the space. Anything else announces itself: `Read`, `Grep`, `Skill`, an
MCP tool. `✎` marks a row the model thought before, `×` marks a call that
failed.

It is **read-only**. It tails the transcript Claude Code already writes and
never modifies anything. It runs on macOS, Linux and Windows.

## Install

**macOS and Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/dontfeedthecode/cc-xray/main/install.sh | sh
```

**Windows** (in PowerShell)

```powershell
irm https://raw.githubusercontent.com/dontfeedthecode/cc-xray/main/install.ps1 | iex
```

Either one downloads the prebuilt binary for your machine (`amd64` or
`arm64`), checks it against the release's checksums, and puts it in
`~/.local/bin` (`~\.local\bin` on Windows). There is nothing to build and
no `sudo` or admin prompt. Then run `ccxray`, in a new terminal on Windows.

- **`ccxray` not found?** On macOS and Linux, the installer doesn't edit your
  shell profile. If `~/.local/bin` isn't on your `PATH`, it prints the line
  to add; for zsh, the macOS default, that is
  `echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc`.
  On Windows, the installer adds the folder to your user `PATH` itself, and
  only terminals opened afterwards see the change.
- **From inside Claude Code,** put `!` in front to run the command in place.
  On Windows `!` runs Git Bash, so wrap the PowerShell line:
  `! powershell -c "irm https://raw.githubusercontent.com/dontfeedthecode/cc-xray/main/install.ps1 | iex"`
- **Options:** `CCXRAY_INSTALL_DIR` installs somewhere else, and
  `CCXRAY_VERSION=vX.Y.Z` pins a release. In PowerShell, set them first, as
  in `$env:CCXRAY_VERSION = 'vX.Y.Z'`.
- **With Go:** `go install github.com/dontfeedthecode/cc-xray/cmd/ccxray@latest`
  puts it in `$(go env GOPATH)/bin`. Binaries for every platform are also on
  the [Releases](https://github.com/dontfeedthecode/cc-xray/releases) page.
- **Uninstall** by deleting the binary.

## Use

Open a second terminal tab **in the directory Claude Code is working in**, and run:

```sh
ccxray
```

It starts empty and attaches to the first session Claude Code writes after
launch — start ccxray, then send your prompt — and keeps up when you `/clear`.
Earlier runs are never replayed; use `--session <id>` for one of those.
It watches the directory it starts in and every parent up to `$HOME`, never
subdirectories: start it where Claude Code runs, or deeper, not above it.
Editors like Zed and VS Code open terminals at the workspace root, so if
Claude Code runs in a subfolder of your workspace, `cd` there first, or pass
`--project <dir>`.

To start it from anywhere, pass `--all`: it then attaches to the first
session written after launch in any project. The directory rule exists so
that a second Claude Code window in another project can't take the panel
over; with `--all` it can, once the session being followed has been quiet
for 20 seconds.

| | |
|---|---|
| `↑` `↓` `k` `j` | scroll a line |
| `pgup` `pgdn` | scroll a page |
| `g` / `G` | jump to the top / follow the live edge |
| `u` | show or hide the usage breakdown by model |
| `c` | clear the panel and wait for the next session to write |
| `q` | quit |

The panel follows new rows like `tail -f`; scrolling up pauses that so
incoming rows don't yank the view away, and `G` resumes.

```
ccxray --project <dir>     # a directory other than the current one
ccxray --all               # any project, wherever ccxray was started
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

**A skill that asked for an effort or model it didn't get** is flagged under
its call. Claude Code drops these silently. A `model:` is skipped in auto
mode when auto mode doesn't support it, or when an allowlist excludes it. And,
undocumented, a skill the model calls itself applies its `effort:` only some
of the time; one you invoke as `/name` has applied it every time. ccxray reads the skill's `SKILL.md` as it is
now, so an edit made after the run can make the flag name the wrong value.

**Δt per action**, so a 20-second step is obvious. **Failures** `×`,
**thinking** `✎`. The model's words before a call sit faintly beneath it.

**Cost, for the turn and the session.** The footer prices the turn (forks
included) and the session so far; `u` breaks both down by model into input,
output, cache reads and cache writes, as `/usage` does. Claude Code only
writes its own total when you leave a session, so a live session's figure is
priced by ccxray from the transcript and marked `~`: it runs a few percent
under `/usage`, which also counts calls Claude Code never writes to the
transcript, such as title generation. Once Claude Code's total is there it
is used as is. Prices are API rates; on a subscription they show what the
tokens would cost, not what you are billed.

A turn ends when you type the next prompt and nothing else — a background
agent reporting back mid-turn does not start a new one. The exception is a
skill forked into the background: while it runs, anything you type joins
the turn that launched it, drawn inline as `▎ your prompt`, so the fork
keeps streaming in place and its return (`▸ returned`) and the model's
answer to it land where you can see them.

## Requirements

- [Claude Code](https://claude.com/claude-code) (validated against `2.1.278`)
- macOS, Linux or Windows, `amd64` or `arm64`

On Windows, use Windows Terminal (the default on Windows 11). If an older
console window draws boxes where the glyphs should be, run with `--ascii`.

The transcript format is internal to Claude Code and changes between
versions, so `ccxray` parses defensively: unknown fields are ignored, a bad
line is skipped rather than fatal, and a version mismatch shows a notice.

## How it works

Claude Code writes every session to
`~/.claude/projects/<slugged-cwd>/<session-id>.jsonl` (under
`%USERPROFILE%` on Windows, where `C:\work\app` slugs to `C--work-app`),
appending as the turn runs. `ccxray` tails that file, tracking a byte offset
so it only parses what is new, and rebuilds the turn from it.

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
that drives a deliberately varied one. Start Claude Code in this directory
and run `/ccxray-demo`. It works through plain shell calls, several named
tools, a deliberate failure, and two skills that move the model and effort
out from under the turn.

It needs [Go](https://go.dev/dl/) (on Windows, `winget install GoLang.Go`),
and it sets up the rest itself. It first checks that ccxray is running. If
it isn't, the skill installs it from this checkout with `go install` when no
`ccxray` is found, prints the command to start it in a second terminal, and
waits for you to say it's running. If Go was installed after Claude Code
started, Claude Code's shell can't see it yet; the skill says so, and
restarting Claude Code from a new terminal fixes it.

To run the checkout without installing it, use `go run ./cmd/ccxray` from
this directory.

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
