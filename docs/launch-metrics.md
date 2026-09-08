# Launch metrics (external)

Agent Undo v0.1 has **no in-product telemetry**. After the public GitHub repository and `v0.1.0` tag exist, track launch from the outside.

Owned by a human. This file is a checklist, not a dashboard.

## Quantitative (where the host provides them)

- GitHub stars
- Forks
- Issues (by [taxonomy](../.github/ISSUE_TEMPLATE/config.yml))
- Clone / download signals GitHub exposes
- X impressions (only if an announcement happens later)
- Demo engagement (people running `examples/setup-demo`)

## Qualitative

- Successful user reports
- Restore failures reported
- Installation problems
- Requested integrations
- Recurring feature requests

**Strongest product signal:** “I almost lost my repo and Agent Undo saved me.”

## After-launch priority

1. Data-loss / correctness bug
2. Installation blocker
3. Platform blocker
4. Repeated workflow limitation
5. High-demand integration
6. Only then a new product capability

Do not build a feature because one person asked once. Look for repeated pain. No speculative roadmap in the first period after release.
