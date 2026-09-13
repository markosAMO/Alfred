# Phase protocol

Every phase follows the same cycle. A phase that skips a step breaks the phases downstream
that depend on it.

## The cycle

```
1  load state          read the pipeline state for this change
2  recover context     context() and recall() for what is already known
3  read inputs         fetch() the artifacts this phase depends on
4  do the work         the part that differs between phases
5  write the document  to the file, from the matching template
6  index               remember() the document under its deterministic key
7  update state        mark the phase completed
8  notify              phase_completed, or error
```

## Ordering rules

**The file is written before memory is updated.** If the run dies between the two, the
artifact still exists and `reindex` recovers the copy. The reverse loses the artifact.

**State is updated after the document exists.** State claiming a phase finished when its
document is missing is worse than no state: `continue` would skip the phase.

**Notification is last.** The user is told a phase finished only once it actually finished.

## Inputs and outputs

A phase declares what it reads and what it writes in its `SKILL.md` frontmatter. It reads
nothing else. A phase that reaches for an artifact it did not declare creates a hidden
dependency that breaks when phases are skipped.

## Skipping

A phase marked `skippable` may be absent. Downstream phases handle a missing input by
falling back to what exists, never by failing.

```
spec with no proposal      the requirement comes from the user's request directly
design with no research    proceed with what architecture.md already states
```

## Failure

A phase that cannot complete leaves state marked `failed` with the reason, writes no
partial document, and raises `error`. A partial document indexed as complete poisons every
phase that reads it afterwards.

## Interaction

A phase in `interactive` mode uses `ask()` and waits. A phase in `confirm` mode uses
`confirm()` before step 4 and stops if the answer is no. A phase in `auto` mode calls
neither, and must not block for input under any circumstance.
