---
name: research
mode: auto
skippable: true
reads: [proposal, architecture, memory]
writes: [research]
document: docs/changes/{change}/research.md
next: [spec]
---

# research

Investigate what is not known before a decision depends on it. Present options with their
consequences. Do not choose.

Choosing is `design`, which reads this document. A research phase that returns one option
has made the decision and hidden it behind an investigation.

## Where it sits

Between `refine` and `spec`. It investigates against the proposal, and what it finds feeds
the requirement that `spec` then writes.

It does not read the specification: the specification does not exist yet. A question that
only appears once the requirement is formal — how to build something already decided —
belongs to `design`, which does its own reading.

## When it runs

```
a library or approach must be chosen and the trade-offs are not known
an external service must be integrated and its constraints are undocumented
the architecture document does not cover the ground this change needs
a previous attempt failed and why is unknown
```

It is `skippable`. Research that confirms what the team already knows costs a phase and
produces a document nobody reads.

## Before searching

```
recall this topic
```

The decision may already have been made and recorded. Re-researching a settled question
produces a second answer that disagrees with the first for no reason anyone can see later.

## What it produces

```markdown
## Question
## Options
### <option>
- how it works
- what it costs
- what it constrains
- how it fails
## Comparison
## Sources
## Open
```

Each option carries how it fails. An option presented only by its strengths is a
recommendation wearing a comparison's clothes.

`Constrains` is the field that matters six months out: what this choice makes harder later.

`Sources` records where each claim came from, including the version it applied to. An
undated claim about a library is unusable the moment the library moves.

`Open` names what could not be resolved, so `design` decides knowing the gap rather than
assuming it was covered.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/research`, update state, notify `phase_completed`.
