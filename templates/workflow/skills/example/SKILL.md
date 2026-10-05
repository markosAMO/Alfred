---
name: {phase}
mode: auto
skippable: false
reads: [{what this phase needs}]
writes: [{what it produces}]
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

What to read, by address. This phase runs as a subagent with an empty context and is handed
paths, never content, so say which paths and what to take from each.

```
recall alfred/area/{area}/...    what a previous change of this kind concluded
```

A recall that finds nothing is a normal result and not a reason to stop.

## The work

The steps, in the order they are done, with the decision each one turns on. State what makes
a step's answer wrong, not only what the step is: a step whose failure is undescribed is one
that gets reported as done.

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
