---
name: apply
mode: confirm
skippable: false
reads: [tasks, spec, design, conventions, review_findings]
writes: [code]
next: [verify]
---

# apply

Execute the task list. One subagent per task, each with an empty context.

This phase dispatches and records. The code is written by the subagents, and this phase
does not write any of it itself.

## Before dispatching

```
recall alfred/area/{area}/review-findings    for the areas the task list touches
```

Past `should fix` findings are passed to the subagent whose task touches the file they are
about, as context rather than as work: a task does what its task says. A finding that is
cheap to honour while already editing that file is honoured; anything larger stays a finding
and is reported, not silently absorbed into an unrelated change.

This is the other half of what `archive` writes those findings for. `review` reads them to
stop repeating itself; this phase reads them because it is the one that can actually fix
them, and the moment it is holding the file open is the only cheap moment there is.

## Confirmation

This phase runs `confirm`. Before dispatching anything:

```
confirm("Implement 8 tasks in login-google", "strict TDD, 3 can run in parallel")
```

It is the first phase that changes the repository, and the last point where the plan can be
rejected while rejecting it is still free.

## Dispatch

Each subagent receives addresses, per `skills/_shared/subagent-protocol.md`.

```
Skill:       .alfred/skills/apply/SKILL.md
Task:        3 of alfred/{change}/tasks
Spec:        alfred/{change}/spec
Design:      alfred/{change}/design
Conventions: docs/code_conventions.md
```

The subagent reads what its task needs. It does not receive the documents inline.

## Running in parallel

Two tasks may run at the same time when **both** hold:

```
no dependency between them, directly or through another task
no file in common, per the files each declares
```

Dependencies alone are not enough. Two independent tasks that both edit `config/routes.rb`
are two subagents writing the same file in the same working tree, and the second write
silently discards the first. Subagents share a checkout; they do not share a lock.

Files that almost every task touches are exclusive by nature and worth naming as such:
route tables, schema dumps, dependency manifests, generated configuration, locale files.
A task touching one of those runs alone.

`phases.apply.max_parallel` caps how many run at once. The limit is about review and
recovery, not machine capacity: when six subagents fail together, finding which one broke
what costs more than the time saved.

When in doubt, run sequentially. A wrong parallel decision costs a silent overwrite that
`verify` may not catch; a wrong sequential decision costs time.

## Detecting a collision

Each subagent reports `files_changed`. If two tasks that ran together report the same file,
the run stops and reports it, even when tests pass.

`tasks` predicted the file lists; the reports are what actually happened. A file that
appears in a report and not in the declaration is a prediction that was wrong, and the next
run of that phase treats it as exclusive.

Files shared with *another change* are not this rule. Subagents of different changes write
different checkouts, so nothing is overwritten; what collides is the branches, later. A task
reaching a file another change owns stops and reports, and which files those are is known
before dispatch, per `skills/_shared/worktree-protocol.md`.

## Writing code

Follow `docs/code_conventions.md` for how this repository writes code, and
`skills/_shared/testing-protocol.md` for what deserves a test.

With `testing.strategy: strict_tdd`, for each task:

```
1  write the test for each scenario the task claims
2  run it, and confirm it fails
3  write the smallest code that satisfies it
4  run it, and confirm it passes
5  refactor with the test still passing
```

Step 2 is the one that carries the value. A test written after the code passes because it
describes what the code does, which is not the same as describing what the specification
asked for. A test that passes before the code exists is testing nothing, and finding that
out at step 2 costs a minute.

With `after`, tests follow the code for the same scenarios. With `none`, no tests are
written and `verify` will report the coverage gap rather than pass silently.

## Scope

A subagent implements its task and nothing else. Code it notices that is wrong but unrelated
is reported in its summary, never fixed.

An unrelated fix inside a feature commit is invisible in review, unreviewable on its own,
and impossible to revert without reverting the feature.

## Returning

```yaml
task: 3
status: completed | failed | blocked
files_changed: [app/controllers/auth_controller.rb, spec/controllers/auth_spec.rb]
summary: one paragraph
reason: only when failed or blocked
```

`files_changed` is what `archive` stages. A file changed and not reported will not be
committed.

## Failure

A failed task stops the tasks that depend on it, which are marked `blocked` and never
attempted. Independent tasks continue: stopping everything because one task failed discards
work that was going to succeed.

State records which task failed and why. `continue` resumes from it rather than restarting
the phase.

A task that fails twice for the same reason stops the phase and raises `error`. Retrying a
third time produces a third variation of the same misunderstanding.

## Completion

Follow `skills/_shared/phase-protocol.md`: update state with per-task results, notify
`phase_completed`.

This phase does not commit. The commit happens in `archive`, once `verify` and `review`
have passed, per `git.granularity: per_feature`.

```
8 tasks: 8 completed, 0 failed. 14 files changed.
```
