# Subagent protocol

The planning phases decide what to do. Subagents do it. A subagent starts with an empty
context, performs one task, and returns.

The clean context is the point: a subagent that never read the previous six tasks cannot
be confused by them, and spends its whole window on the one task it owns.

## What a subagent receives

Addresses, not content. Each one is a locator the orchestrator already resolved, per
`skills/_shared/orchestrator-protocol.md`.

```
Task:        3 of docs/changes/login-google/tasks.md
Skill:       .alfred/skills/apply/SKILL.md
Spec:        docs/changes/login-google/spec.md
Design:      docs/changes/login-google/design.md
Conventions: docs/code_conventions.md
```

A locator is read as what its shape says: a path is a file, a key is a memory entry fetched
through `recall` then `fetch`. The subagent does not read the configuration to find out
which it is holding, and does not go looking in the other store when one comes back empty —
an empty result is reported, never worked around.

A subagent works where its session is. A change running in its own worktree runs in a
session whose working directory is that worktree, per `worktree-protocol.md`, so the
checkout is not an address the subagent has to be given or keep hold of.

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
context: 49k tokens
memory_conflicts: [alfred/login-google/design]
blocked_by: task 2
reason: only when failed or blocked
```

`files_changed` is what lets `git.stage: files_changed_by_alfred` stage precisely the work
Alfred did, and nothing the user left in progress.

`context` is what the subagent spent. It is known to the subagent and to nobody else, and it
is the number that tells you whether a phase is worth what it does.

`memory_conflicts` names the entries the backend refused to settle, per
`memory/CONTRACT.md`. A subagent cannot resolve one and does not try; `archive` collects
them.

## Dependencies

`tasks` declares dependencies. A subagent is dispatched only once its dependencies are
`completed`.

A task whose dependency failed is marked `blocked`, never attempted. Running it anyway
produces code written against an interface that does not exist.

## Concurrency

Subagents of one change share one checkout. There is no file locking between them, so two that write the
same file produce one silent winner rather than a conflict anyone notices.

Running together therefore requires two conditions, not one: no dependency between the
tasks, and no file in common. `tasks` declares the file list that makes the second
decidable.

A collision that happens anyway is detected afterwards, from the `files_changed` each
subagent reports: the same file in two concurrent reports stops the run, even when the
tests pass.

Subagents of different changes never share a checkout: each change started alongside
another runs in its own worktree, and the file rule above does not apply between them.

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
