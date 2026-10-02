---
name: spec
mode: auto
skippable: false
reads: [proposal, diagnosis, architecture, master_specs, memory]
writes: [delta_spec]
document: docs/changes/{change}/spec.md
next: [design]
---

# spec

Turn intent into requirements that can be verified. Every scenario written here becomes a
test, and `verify` checks the implementation against this document and nothing else.

This is the only phase that decides what the system must do. `design` decides how, `apply`
builds it, `verify` measures against this. A vague requirement here is not caught later:
it is inherited by every phase downstream, which will all agree with it.

## Inputs

In order of preference, whichever exists:

```
alfred/{change}/proposal     from refine
alfred/{change}/diagnosis    from diagnose, when the change is a bug
alfred/{change}/input/request when routing went straight here
```

The third is a path like the other two. A request that reaches this phase as text in its
prompt has been carried through the orchestrator to get here, and this phase is then the one
writing it down — per `skills/_shared/external-inputs.md`, it is written when the route is
accepted, before any phase is dispatched.

Read `architecture.md` for the vocabulary of the system, so a requirement uses the terms
the project already uses rather than inventing parallel ones.

## Documenting on demand

A requirement that modifies existing behaviour needs the existing behaviour written down.

```
search master_specs for the requirement being changed
  found      -> write a MODIFIED delta against it
  not found  -> run explore scoped to that area first, then write the delta
```

`explore` derives the current specification of that area from the code and its tests. It
covers the area under change, never the whole repository: a codebase documents itself as it
is worked on, and documenting two hundred untouched features produces specifications
nobody reads.

A requirement that adds behaviour with no existing counterpart is `ADDED` and needs no
lookup.

## Writing the delta

Follow `skills/_shared/document-style.md`. Sections are `## ADDED Requirements`,
`## MODIFIED Requirements`, `## REMOVED Requirements`.

A requirement states a single obligation in the system's own vocabulary.

```markdown
### Requirement: Google sign-in
The system SHALL authenticate users through Google OAuth.
```

One obligation per requirement. Two obligations joined by "and" produce a scenario that
passes when half the behaviour works.

`MODIFIED` states the new behaviour and what it replaces, so review sees the difference
without opening the master spec.

```markdown
### Requirement: Session expiry
Sessions SHALL expire after 30 days of inactivity.
Previously: sessions expired after 7 days.
```

## Scenarios

Every requirement carries at least one success scenario and one failure scenario. A
requirement with only the happy path is incomplete, per
`skills/_shared/testing-protocol.md`, and produces a feature that was demonstrated rather
than tested.

```markdown
#### Scenario: successful sign-in
- GIVEN a user with a valid Google account
- WHEN they complete the OAuth consent screen
- THEN a session is created
- AND the user lands on the dashboard

#### Scenario: consent denied
- GIVEN a user who declines the consent screen
- WHEN they return to the callback
- THEN no session is created
- AND they see an explanation of what failed
```

A scenario names observable states and observable outcomes. "THEN the service handles it
correctly" cannot fail a test, which means it cannot pass one either.

Scenario titles become test names, so they are written to be read in a failure report.

## Open questions

The proposal may carry unanswered questions. They are not ignored and they are not invented
away.

```
does the gap prevent writing a verifiable scenario?
  yes  -> raise needs_input and wait
  no   -> record it under Assumptions and continue
```

This phase runs in `auto` mode and does not interview. Raising `needs_input` hands one
specific blocking question back rather than reopening the interview.

An assumption recorded here is visible to `design` and `review`. An assumption made
silently becomes a requirement nobody agreed to.

## What this phase does not do

No implementation detail. No file names, no class names, no library choices, no database
schema. Those are `design`, and a requirement that names them cannot be satisfied any other
way, which turns one valid solution into the only permitted one.

No estimates, no task breakdown, no priorities.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/spec`, update state, notify `phase_completed`.

Report the counts, because they are what the user checks at a glance:

```
spec written: 3 requirements, 7 scenarios, 1 assumption
```
