# Orchestrator protocol

The orchestrator decides what happens next and delegates it. It does no work inline.

It is the only participant that lives for the whole run. Everything it reads stays in its
context until the run ends, so what it reads is a budget, not a convenience.

## What it holds

```
the user's request
.alfred/config.yaml
.alfred/state/{change}.yaml
.alfred/skill-registry.md
```

Roughly two pages. That is the entire working set.

## What it never reads

Specifications, designs, task lists, proposals, source files, diffs, test output, and the
contents of memory entries.

A subagent reads those, uses them, and disappears with them. The orchestrator that reads
them carries them for the rest of the session.

After eight tasks, an orchestrator that read every artifact holds eight specifications and
eight diffs, hits compaction, and loses the one thing nothing else can rebuild: the plan
and where the run is inside it. Every document it dropped is still on disk; the thread of
the run is not.

## Delegation

The orchestrator passes addresses.

```
Skill:  .alfred/skills/apply/SKILL.md
Task:   3 of alfred/login-google/tasks
Spec:   alfred/login-google/spec
Design: alfred/login-google/design
```

It does not open any of them to decide what to send. `tasks.md` is read by the phase that
dispatches from it, and the orchestrator learns the count and the dependency order from
state, not from the document.

## Reporting

A subagent returns a one-paragraph summary and a list of changed files, per
`subagent-protocol.md`. The orchestrator relays that. It does not fetch the diff to check,
and does not paste subagent output into its own context to reason about it.

`verify` and `review` exist to judge the work. An orchestrator re-reading the code to form
its own opinion duplicates them and pays for it in context that the rest of the run needs.

## Inline work

None. Not a one-line fix, not a quick file read to answer a question, not a rename that
would be faster to do directly.

The exception that proves it: reading state and the registry, which are the orchestrator's
own bookkeeping.

A single inline edit is cheap. The habit is not: the orchestrator that edits one file reads
that file, then the file next to it, then the test, and the working set stops being two
pages.
