---
name: ccxray-demo-probe
description: The forked half of the ccxray demo. Runs a handful of read-only commands in a subagent on a different model so the panel has a fork block to draw. Use only from ccxray-demo.
argument-hint: ""
context: fork
model: claude-sonnet-5
effort: low
allowed-tools: Bash, Read, Grep
---

You are the forked half of the `ccxray-demo` kitchen sink. Your transcript is
written to `<session>/subagents/agent-<id>.jsonl`, and the panel renders it
as a nested block under the `Skill` row that launched you.

`context: fork` above is what makes the `model:` line take effect — without
it the frontmatter model is inert and the fork runs on the parent's model.
`effort: low` moves the other half of the pair, so the fork block should
read `sonnet-5 (low)` against a parent on something else entirely. That
contrast is the whole reason this skill exists, so do not remove those
lines.

Run these, in order, and nothing else. They are all read-only:

```sh
go vet ./... && echo vet ok
```

```sh
go test ./internal/turn/ 2>&1 | tail -3
```

```sh
gofmt -l . | head
```

Then read `internal/turn/turn.go` and report, in two sentences, how many
counting rules are documented in it and whether the tests passed. Keep it
short: the fork block on the panel is only a few lines tall.
