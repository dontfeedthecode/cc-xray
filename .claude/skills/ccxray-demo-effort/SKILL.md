---
name: ccxray-demo-effort
description: The in-thread half of the ccxray demo. Declares an effort level but does not fork, so the panel can show whether effort moves without a subagent. Use only from ccxray-demo.
argument-hint: ""
effort: max
allowed-tools: Bash
---

This skill exists to answer one question the panel beside you is the only
way to answer: **does `effort:` in frontmatter take hold without
`context: fork`?**

`model:` does not — that is settled, and `ccxray-demo-probe` demonstrates
it. `effort:` is a separate key, so it may behave differently. This skill
declares `effort: max` and deliberately does *not* fork.

Run these two, and nothing else:

```sh
git -C . rev-parse --short HEAD
```

```sh
go test ./internal/record/ 2>&1 | tail -2
```

Then look at the ccxray panel and report what the EFFORT part of the MODEL
column says on those two rows compared with the rows above them:

- If it changed, `effort:` applies in-thread and the panel should have drawn
  a `STATE` row with an `effort: … → max` detail line under it.
- If it did not, `effort:` shares the fork requirement with `model:`, and
  the panel correctly shows no change.

Either answer is useful. Say plainly which one you saw, and do not guess
from the frontmatter — read the panel, or read the `effort` and
`perTurnEffort` fields on the last few assistant records of the transcript:

```sh
tail -40 "$(ls -t ~/.claude/projects/*/*.jsonl | head -1)" \
  | python3 -c 'import sys,json
for l in sys.stdin:
    try: r=json.loads(l)
    except: continue
    if r.get("type")=="assistant":
        print(r.get("effort"), r.get("perTurnEffort"), (r.get("message") or {}).get("model"))'
```
