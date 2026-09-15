---
name: refine
mode: interactive
skippable: true
entry_point: new_feature
reads: [architecture, conventions, tracker_card, memory]
writes: [proposal]
document: docs/changes/{change}/proposal.md
next: [research, spec]
---

# refine

Interview the user until the request is understood well enough to specify. Write nothing
else: no specification, no design, no code.

This phase exists because a request arrives shorter than the thing being requested. "Add
Google login" is eight words describing a week of work, and every gap left in it is filled
by the model guessing.

## When the orchestrator routes here

A request goes to `refine` when any of these hold. Otherwise it goes straight to `spec`.

- The failure behaviour is unstated. The request says what should happen when it works.
- A domain term appears that is not defined in `architecture.md` or memory.
- More than one component or repository is affected.
- It requires an architectural decision that has not been made.
- There is no way to tell, from the request alone, whether the result is correct.

A request carrying none of these is already specifiable, and interviewing the user about
it wastes their time.

When the request arrives as a tracker card, the same criteria apply to the card body.

The decision is proposed and confirmed, never applied silently. The orchestrator names the
signals it matched and offers to skip straight to `spec`. See `skills/_shared/routing.md`.

## Materialise the inputs first

Before anything else, bring in whatever arrived as a reference: tracker cards, URLs,
documents in other systems.

```
read each source once
write it in full to docs/changes/{change}/inputs/
index it under alfred/{change}/input/{slug}
```

Follow `skills/_shared/external-inputs.md`. Nothing after this phase fetches anything: the
interview, `spec`, `design` and every subagent read the materialised text.

## Before asking anything

Read what is already known, and never ask for it.

```
fetch alfred/{change}/input/*        the material just materialised
fetch alfred/project/architecture
fetch alfred/project/conventions
recall the feature area, for prior decisions and related changes
```

Asking which database the project uses, when `architecture.md` states it, teaches the user
that answering carefully is pointless.

Open every round by naming what was assumed from these sources, so a wrong assumption is
corrected instead of inherited.

## The interview

Questions are asked in rounds, grouped by topic. One `ask()` per round carrying three or
four questions, never one question at a time.

Each round is answered before the next is asked. Later rounds depend on earlier answers:
asking about integration before the scope is settled produces questions about work that
turns out to be excluded.

### Round 1 — Problem and scope

- What problem does this solve, and for whom?
- What does the user do today instead?
- What is explicitly out of scope?
- Is this new behaviour, or a change to behaviour that already exists?

### Round 2 — Behaviour

- What happens when it works? Walk through it.
- What can go wrong, and what should happen then?
- Which edge cases matter, and which are acceptable to ignore?
- What must never happen?

The failure path is not optional. A round that produces only the happy path is not
finished, because `testing-protocol.md` requires both and the scenarios come from here.

### Round 3 — Integration and direction

- Which parts of the system does this touch?
- Which external services are involved?
- What data is read, written or stored?
- Is there an approach you already want, or one you want ruled out?

The last question is the only one that collects a technical decision, and it collects the
user's, never the model's. An answer to it is a constraint on `design`, not a design.

Leave it unanswered rather than filling it in. A direction the user did not state is a
decision `design` should be making with the specification in front of it.

For multi-repository work, this round determines which repositories are affected and is
the input to workspace `design`.

### Round 4 — Constraints and acceptance

- How will we know it is done?
- Are there performance, security or compliance constraints?
- Is there a deadline or a dependency on other work?
- What would make you reject this once built?

## Stopping

Stop when a round produces nothing that changes the proposal. Continuing past that point
collects detail that belongs in `design`.

Stop early when the user says to. A partial proposal with its gaps marked is more useful
than a complete one built on invented answers.

Never fill a gap by assuming. An unanswered question is written into the proposal as an
open question and carried into `spec`.

## What it writes

```markdown
## Problem
## Proposed change
## Scope
### Included
### Excluded
## Behaviour
### Happy path
### Failure paths
### Edge cases
## Integration
### Components affected
### External services
### Data
## Architectural direction
## Constraints
## Acceptance
## Open questions
## Assumptions
```

`Architectural direction` holds only what the user decided, each line with the reason they
gave. It is empty when they decided nothing, and an empty section is the normal case.

```markdown
## Architectural direction
- Events, not polling. The consumer cannot be blocked by the producer.
- Reuse the existing notification service rather than adding a second one.
```

These are constraints `design` inherits, not a design. They name a direction and a reason;
they name no interface, no schema and no library.

`Assumptions` records what was taken from `architecture.md` or memory rather than asked.
`spec` treats those as inherited, not as decided, and a wrong one surfaces there instead of
in the code.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/proposal`, update state, notify `phase_completed`.

The proposal is prose describing intent. It contains no `SHALL`, no scenarios and no delta
sections. Those belong to `spec`, and writing them here produces a requirement nobody
reviewed as one.
