---
name: ccxray-demo
description: Drive one turn that exercises every row ccxray can draw — plain tool calls, several named tools, a deliberate failure, a forked skill and a model change — after checking ccxray is installed and running, and installing it from this checkout if not. Use when the user asks to demo, smoke-test or kitchen-sink ccxray, to see what the panel looks like under load, or to get ccxray set up to try it.
argument-hint: ""
allowed-tools: Bash, Read, Grep, Glob, Write, Skill, AskUserQuestion
---

A kitchen-sink turn for the panel running beside this one. Every step below
exists to put a different kind of row on screen, so run them **in order, as
one turn**. Apart from the preflight in step 0, do not stop to ask for
confirmation — the user asked for the whole sequence when they invoked this
skill.

Keep each step small. The point is the shape of the turn, not the output.

Run every shell block with the `Bash` tool, on Windows too, where it is Git
Bash: they are POSIX shell, and PowerShell would reject most of them.

## 0 — preflight

Before anything else, check that ccxray is running and that this session can
run Go, which the later steps need:

```sh
sh .claude/skills/ccxray-demo/preflight.sh
```

Describe it as "Check ccxray is running". Its first line says what to do:

- `ready` — carry on with step 1.
- `start` — ccxray is not running. If the output says it was installed, say
  so in one line. Show the user the start command it printed, in a code
  block of its own (a `powershell` block on Windows, `sh` elsewhere), plus
  the `--all` line as the way to start it from any folder. Then ask with
  `AskUserQuestion` whether ccxray is running, with the options "It's running"
  and "Stop the demo". On "It's running", carry on with step 1 in this same
  turn: the panel attaches at the next write and draws the whole turn,
  preflight included. On "Stop the demo", stop.
- `restart` or `no-go` — relay the rest of the output in a few plain
  sentences and stop. The demo cannot work until Claude Code can find Go,
  and a running process cannot pick up a PATH changed after it started.
- `error`, or anything else — show the output and stop.

## 1 — plain shell calls

These draw the common case: no tool name, description only.

```sh
mkdir -p .ccxray-demo && echo "scratch" > .ccxray-demo/note.txt
```

```sh
go build ./... && echo build ok
```

```sh
git -C . log --oneline -3
```

Give each `Bash` call a clear `description`: it is the only thing the row
shows, and writing vague ones is the fastest way to make the panel useless.

## 2 — named tools

Each of these announces itself in the ACTION column, so the row stands out
against the shell calls around it.

- `Read` — read `go.mod`.
- `Glob` — find `internal/**/*_test.go`.
- `Grep` — search for `func Render` under `internal/ui`.
- `Write` — write a one-line file to `.ccxray-demo/written.txt`.

## 3 — a failure

Run exactly this, and expect it to fail. It is a read of a path that does not
exist, so nothing is at risk:

```sh
ls /ccxray-demo-no-such-path
```

The row should pick up the `×` marker and turn red. Say in one line that the
failure was deliberate, then carry on — do not try to fix it.

## 4 — a forked skill, a model change and an effort change

Invoke the `ccxray-demo-probe` skill. It runs with `context: fork`, which is
the only way a skill's frontmatter `model:` actually takes effect, and it
also declares `effort: low`. The panel should show a nested fork block
running on `sonnet-5 (low)` against whatever this thread is on — both halves
of the MODEL column moving at once.

## 5 — effort without a fork

Invoke the `ccxray-demo-effort` skill. It declares `effort: max` and does
*not* fork. `effort:` only takes hold when the skill is resolved before the
turn's first request, and this call comes many requests in, so the panel
should show **no** change. Report what it actually shows rather than
assuming. To see effort move, the user can run `/ccxray-demo-effort` on its
own as the next prompt.

## 6 — clean up and answer

```sh
rm -rf .ccxray-demo
```

Finish with two or three sentences describing what the panel showed: how many
actions, which tools were named, whether the failure and the fork rendered.
That closing message is itself a row — the `answer` row — so the turn ends
with one of each.
