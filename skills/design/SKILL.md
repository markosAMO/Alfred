---
name: design
mode: interactive
skippable: false
reads: [spec, diagnosis, architecture, conventions, research, memory]
writes: [design]
document: docs/changes/{change}/design.md
next: [tasks]
---

# design

Decide how the specification will be satisfied. One approach, chosen deliberately, with the
alternatives that were rejected recorded next to it.

`spec` says what the system must do. This phase says how this codebase will do it. It
writes no code.

## Before deciding

```
fetch alfred/{change}/spec
fetch alfred/{change}/proposal        for Architectural direction, when refine ran
fetch alfred/project/architecture
fetch alfred/project/conventions
recall this area, for decisions already made
```

The architecture document constrains this phase. A design that contradicts it is either
wrong or is proposing to change the architecture, and those are different conversations.
Proposing the second one is legitimate; doing it without saying so is not.

## Inherited direction

`Architectural direction` in the proposal is what the user already decided during `refine`.
It enters this phase the same way `architecture.md` does: as given.

It is not reopened. A user who settled an approach in the interview and is asked about it
again learns that answering carefully changes nothing, and starts answering carelessly.

Where an inherited direction turns out to conflict with the specification or with
`architecture.md`, say so and ask. Stating the conflict is the job; resolving it silently in
either direction is not.

```
ask("You chose events over polling during refine, but the spec requires a
     synchronous confirmation to the caller. Which one gives?",
    ["keep events, confirm asynchronously", "synchronous call", "other"])
```

## Two scopes

At workspace level, this phase decides **which repository does what**, and produces the
allocation that workspace `tasks` distributes. It does not decide implementation inside any
repository.

At repository level, it decides **how this repository implements its part**.

Both exist because they answer different questions. See
`skills/_shared/workspace-protocol.md`.

## Interaction

This phase runs `interactive`, and asks only when an answer changes the design.

```
ask when   the specification admits two approaches with materially different consequences
           an unrecorded architectural decision is required
           a trade-off costs something the user should choose to pay

do not ask when   architecture.md or conventions already answer it
                  one approach is clearly better and the reason can be written down
                  the choice is reversible and cheap
```

Asking about a decision already recorded teaches the user that the documents are not read.
Deciding silently on something expensive to reverse hands them a consequence they never
agreed to.

## What it writes

```markdown
## Approach
## Components affected
## Interfaces
## Data
## Trade-offs considered
## Risks
## Out of scope
```

`Approach` is the chosen design in prose, stated so a reader can tell whether it satisfies
each requirement.

`Interfaces` names the contracts that change or appear: signatures, endpoints, events,
message shapes. These are what other tasks depend on, so `tasks` reads this section to
order the work.

`Trade-offs considered` records what was rejected and why. Without it the next change
reopens a settled decision, because nothing recorded that it was settled. One line per
alternative is enough.

`Out of scope` names what this design deliberately does not solve, so `review` does not
report it as missing.

## Decisions that outlive the change

A decision that will constrain future work does not belong here. It belongs in
`architecture.md`, and this phase proposes the addition.

```
"use Postgres for this feature"        this document
"all persistence goes through Postgres" architecture.md
```

Buried in a change document, a durable decision is invisible to every future change and
gets re-decided differently.

## What this phase does not do

No code, no file contents, no task breakdown. Naming a file that will be created is fine;
writing what goes in it is `apply`.

No new requirements. Behaviour that the specification does not describe is out of scope,
and discovering that the specification is incomplete escalates to `spec` rather than being
filled in here.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/design`, update state, notify `phase_completed`.
