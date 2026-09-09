# Launch metrics (external)

Agent Undo v0.1 has **no in-product telemetry**. After the public GitHub repository and `v0.1.0` tag exist, track launch from the outside.

Owned by a human. This file is a checklist, not a dashboard. Issue counts do not represent users who bounced and never filed.

v0.1 identity: **session rollback for wrapped terminal agents**. That is not the same category as **reversible AI coding sessions**. Do not build the second one until users show they want it badly enough to change the attachment model.

## Quantitative (where the host provides them)

- GitHub stars
- Forks
- Issues (by [taxonomy](../.github/ISSUE_TEMPLATE/config.yml))
- Clone / download signals GitHub exposes
- X impressions (only if an announcement happens later)
- Demo engagement (people running `examples/setup-demo`)

Stars, demo praise, architecture comments, and one-off Cursor requests are **secondary**.

## Qualitative

- Successful user reports
- Restore failures reported
- Installation problems
- Requested integrations
- Recurring feature requests
- Conversations (including people who tried once and left)
- Repeated support questions
- Explicit requests for editor attachment

When triaging, label attach mode from the issue form (not a run-count):

- `attach:terminal` — wrapped with `agent-undo run`
- `attach:editor` — Cursor or other editor-native agent, not wrapped

A user who never returns is invisible. Do not treat GitHub issue mix as the market mix.

**Strongest positive signal:** “Agent Undo saved me from a bad agent session.”

**Strongest negative signal:** “Used once for the demo, then went back to git.”

## Product-learning question

After launch, evaluate Agent Undo on:

> Did the user reach for Agent Undo the **second** time their AI agent made a mess?

The loop we want:

```text
agent makes mess
        ↓
user remembers Agent Undo
        ↓
user uses Agent Undo
        ↓
undo works
        ↓
trust increases
        ↓
user uses it again
```

## Listen hypotheses (not a roadmap)

These may be reordered or dropped by repeated evidence. Do not implement from this list.

1. Last-session default (`undo` / `diff` / `show` with no id)
2. Human-readable session list (name, time ago, path summary)
3. Partial / path rollback
4. Editor-native attachment

Example reorder: wrapped users stay but hate ids → (1) matters. Interested users refuse to wrap → (4) may jump first. Big sessions create fear of whole-tree undo → (3) may become critical.

## After-launch priority

1. Data-loss / correctness bug
2. Installation blocker
3. Platform blocker
4. Repeated workflow limitation
5. High-demand integration
6. Only then a new product capability

Do not build a feature because one person asked once. Look for repeated pain. No speculative roadmap in the first period after release.
