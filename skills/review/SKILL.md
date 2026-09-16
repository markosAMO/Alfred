---
name: review
mode: auto
skippable: false
agent: separate_from_author
reads: [design, conventions, architecture, code]
writes: [review_report]
document: docs/changes/{change}/review-report.md
next: [archive]
---

# review

Answer one question: is this code sound?

Not whether it satisfies the specification. `verify` established that, and repeating it here
spends a review on work already done.

## Runs alongside verify

Dispatched at the same time as `verify`. Neither reads the other's report and neither
writes code, so they cannot interfere.

Do not wait for the verification result, and do not assume it. A review is about whether
the code is sound, which does not change based on whether a scenario passed.

## A separate agent

This phase runs in a context that never saw the code being written. The agent that wrote it
cannot review it: the reasoning that produced a mistake is the reasoning that would have to
catch it, and it already agreed with itself once.

The model is at least as strong as the one that implemented, per `docs/models.md`. A
reviewer weaker than the implementer approves everything, and the phase becomes ceremony.

## What it reads

```
the diff of the change
docs/code_conventions.md
alfred/{change}/design
alfred/project/architecture
```

The design is read to know what was intended, including its `Out of scope` section. Work
deliberately excluded is not reported as missing.

## What it looks for

```
conventions      does it follow docs/code_conventions.md
correctness      error handling, edge cases, resource cleanup, concurrency
simplicity       is there a materially simpler shape with the same behaviour
duplication      does this already exist in the codebase
coupling         does it reach past the boundaries the architecture draws
security         input handling, authorisation, secrets, injection surfaces
readability      will this be understood by someone who was not here
```

## Severity

Every finding carries one, because a report where everything matters equally is a report
nobody acts on.

```
blocking     ships a defect, a security hole, or violates the architecture
should fix   real problem, does not have to stop this change
suggestion   preference, taste, or a possible future improvement
```

Only `blocking` findings stop the change. A phase that blocks on preferences trains the
user to skip the phase.

## Findings

A finding names the file, the line, what is wrong, and what happens as a result.

```markdown
### blocking · app/controllers/auth_controller.rb:42
The OAuth state parameter is not compared before creating the session.
A forged callback creates a session for an account the caller does not control.
```

A finding without a consequence is an opinion. If the consequence cannot be stated, the
finding is a `suggestion` at most.

Rewriting the code is not this phase's job. Findings describe the problem; `apply` fixes it.

## What it does not do

No re-verification of scenarios. No new requirements. No style rewrites that
`docs/code_conventions.md` does not ask for.

Reviewing code the change did not touch produces findings nobody asked for and buries the
ones about the change.

## What it writes

```markdown
## Result
## Blocking
## Should fix
## Suggestions
```

## Failing

Blocking findings return the change to `apply` with the findings attached. Only the tasks
owning the affected files are re-dispatched.

`should fix` and `suggestion` findings are recorded and do not block. `archive` carries
them into memory so they surface when that area is touched again.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/review-report`, update state, notify `phase_completed` or `error`.

```
review: 0 blocking, 2 should fix, 3 suggestions
```
