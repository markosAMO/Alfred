---
name: {phase}
mode: {mode}
skippable: false
reads: []
writes: [{phase}]
document: docs/changes/{{change}}/{phase}.md
next: [{next}]
---

# {phase}

{purpose}

## Inputs

What this phase reads, by address, and what it does when an input is missing.

## The work

The steps of this phase, and why each constraint exists.

## Returning

```yaml
phase: {phase}
status: completed | failed
summary: one paragraph
```

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it, update state,
notify `phase_completed`.
