---
name: {phase}
mode: auto
skippable: false
next: [{the phase that usually follows}]
---

# {phase}

The one question this phase answers, in a sentence. A phase that answers two questions is
two phases, and a route can leave neither of them out.

`mode` is `auto` when the phase starts on its own and does not talk, `interactive` when it
asks the user and waits for an answer, `confirm` when it asks before it starts.
`skippable` says whether a route may leave it out. The model this phase runs on is not here:
it is `model` on this phase's entry in `workflow.json`, which is what registration reads.

## Before starting

What to take from each artifact this phase is handed. Which artifacts those are is not
here: it is `reads` and `recall` on this phase's entry in `workflow.json`, and the
orchestrator passes one locator for each. Every handed artifact ends with a `## Handoff`;
start from it, per `skills/_shared/phase-protocol.md`.

```
the {phase-1} artifact    what to take from it
```

## The work

The steps, in the order they are done, with the decision each one turns on. State what makes
a step's answer wrong, not only what the step is: a step whose failure is undescribed is one
that gets reported as done.

## What it leaves

This phase's artifact, named after the phase, at the locator it was handed. Name its
sections, and end it with `## Handoff`: what the next phase would otherwise pay to find
again.

## Scope

What this phase does not do, and which phase does it instead. A phase that fixes what it
noticed on the way makes a change nobody reviewed.

## Completion

What this phase returns. The orchestrator records these fields and relays them; a phase that
returns prose instead leaves it nothing to record.

```yaml
status: completed | failed | blocked
summary: one paragraph
reason: only when failed or blocked
```
