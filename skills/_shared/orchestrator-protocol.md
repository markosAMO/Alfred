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

The configuration is in that list for a reason beyond its own settings: it is what the
orchestrator resolves every artifact locator from, below.

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

The orchestrator passes addresses, already resolved.

```
Skill:  .alfred/skills/apply/SKILL.md
Task:   3 of docs/changes/login-google/tasks.md
Spec:   docs/changes/login-google/spec.md
Design: docs/changes/login-google/design.md
```

Each of those is a **locator**, and resolving it is the orchestrator's job alone. It holds
`.alfred/config.yaml` already, so it is the only participant that knows what
`memory.documents` says; a phase's context is empty and would have to read the configuration
to find out.

```
keep, ephemeral   a path under paths.changes
pointer           a key, alfred/{change}/{artifact}
```

A phase is never told the mode and never asked to work it out. Two things follow. Adding a
storage mode changes this resolution and nothing in any phase. And a phase can never
disagree with the repository about where its inputs are — the disagreement would be silent,
because reading the wrong store returns an empty result that looks exactly like an artifact
nobody wrote.

An artifact that does not exist is passed as `<unresolved>`, not omitted. The phase then
reports a blocker naming it, instead of treating a missing line as an optional input it may
proceed without. See `Locators` in `memory/CONTRACT.md`.

It does not open any of them to decide what to send. `tasks.md` is read by the phase that
dispatches from it, and the orchestrator learns the count and the dependency order from
state, not from the document.

## The run is a session

The orchestrator brackets the run in memory: it opens a session before the first phase and
closes it after the last, and it records the user's request before any phase derives
anything from it.

```
at the start   open the session, then record the request
at the end     close the session
```

It is the orchestrator's job because it is the only participant that lives for the whole
run. A phase doing it would open and close a session per phase, which groups nothing, and a
phase recording the request would be recording something it did not see — the request
reached it already written down, per `skills/_shared/external-inputs.md`.

Left undone, saves attach to whatever session the backend last had open. Observed in a
seven-phase run: every artifact attached to a session from two days earlier, so nothing
could retrieve the run as a unit and a recall of recent context returned the wrong days.

These are the orchestrator's only memory calls. It stores no artifact and reads no entry's
contents, per `What it never reads` above.

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
