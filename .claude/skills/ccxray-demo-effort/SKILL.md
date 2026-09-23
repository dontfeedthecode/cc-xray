---
name: ccxray-demo-effort
description: The in-thread half of the ccxray demo. Declares effort: max without forking. Run from ccxray-demo, or on its own as /ccxray-demo-effort to watch effort change in-thread.
argument-hint: ""
effort: max
allowed-tools: Bash
---

This skill declares `effort: max` and deliberately does *not* fork, to show
the rule for `effort:` in frontmatter: it applies in-thread, but only when the
skill is resolved before the turn's first request.

- Run as `/ccxray-demo-effort` on its own, the panel's MODEL column should
  read `(max)` on the rows below.
- Called from `ccxray-demo` partway through a turn, it should not change.

Run these two, and nothing else:

```sh
git -C . rev-parse --short HEAD
```

```sh
go test ./internal/record/ 2>&1 | tail -2
```

Then report in one sentence what effort the MODEL column shows on those two
rows compared with the rows above them. Read it off the panel; do not infer
it from this frontmatter.
