# Subagent protocol

The planning phases decide what to do. Subagents do it. A subagent starts with an empty
context, performs one task, and returns.

The clean context is the point: a subagent that never read the previous six tasks cannot
be confused by them, and spends its whole window on the one task it owns.

## What a subagent receives

Addresses, not content.

```
Task:       3 of alfred/login-google/tasks
Skill:      .alfred/skills/apply/SKILL.md
Spec:       alfred/login-google/spec
Design:     alfred/login-google/design
Conventions: docs/code_conventions.md
```

The subagent fetches what its task needs. Pasting the full spec into the prompt spends the
clean context before the work starts, and hands it six thousand tokens to find the one
paragraph that applies.

Skills are passed as paths resolved from `paths.skill_registry`, so a subagent reads the
skill as installed in that repository rather than a copy pasted into its prompt.

## What a subagent returns

```yaml
task: 3
status: completed | failed | blocked
files_changed: [app/controllers/auth_controller.rb, spec/controllers/auth_spec.rb]
summary: one paragraph
blocked_by: task 2
reason: only when failed or blocked
```

`files_changed` is what lets `git.stage: files_changed_by_alfred` stage precisely the work
Alfred did, and nothing the user left in progress.

## Dependencies

`tasks` declares dependencies. A subagent is dispatched only once its dependencies are
`completed`.

A task whose dependency failed is marked `blocked`, never attempted. Running it anyway
produces code written against an interface that does not exist.

## Failure

A failed task stops dispatch of everything that depends on it, leaves the rest untouched,
and is recorded in state under `tasks.blocked`. `continue` resumes from the failed task
rather than restarting the phase.

## Verification is a separate agent

`review` runs in a context that never saw the code being written. An agent reviewing its
own output approves its own assumptions, because the reasoning that produced the mistake is
the reasoning that would have to catch it.

`verify` answers "does this satisfy the spec". `review` answers "is this code sound". They
are different questions and stay in different agents.
