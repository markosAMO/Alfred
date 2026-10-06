---
change: {change}
phase: review
reviewer_context: separate_from_author
---

# Review — {change}

## Result
`passed` or `blocked`.

## Blocking
Ships a defect, a security hole, or violates the architecture. These stop the change.

### {severity} · {file}:{line}
{what is wrong}
{what happens as a result}

A finding without a consequence is an opinion.

## Should fix
Real problems that do not have to stop this change. Carried into memory by `archive`.

## Suggestions
Preference or possible future improvement. Never blocking.

## Handoff

{Where the next phase starts: paths with lines, commands with what they printed, what was ruled out, what is still open. Fifteen lines at most, or `none`.}
