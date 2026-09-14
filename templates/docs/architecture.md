---
document: architecture
scope: project
written_by: [init, explore]
updated_by: [design]
---

# Architecture

What is already decided. Read by every phase, and by `design` before it decides anything.

This describes the project, not any feature. A statement here constrains every change that
comes after it.

## Stack

Languages, frameworks, runtimes and versions. What runs where.

## Layers and boundaries

The layers, and which may call which. State the direction, not only the names.

```
controllers -> services -> repositories -> database
controllers never reach a repository directly
```

Known violations are listed as exceptions, with where they are. A boundary crossed in three
places is a boundary with three exceptions, not an absent one. `review` enforces what is
written here, so an unwritten boundary is unenforceable.

## Persistence

Where state lives and why there. Databases, caches, queues, files, and what is authoritative
when two disagree.

## External services

What is consumed, what it is used for, and what happens when it does not respond. The
failure behaviour is the part that gets forgotten and the part that matters at three in the
morning.

## Cross-cutting

Authentication and authorisation, logging, error handling, configuration and secrets,
background work. How each is done here, once, so no feature invents its own.

## Decisions

One entry per decision that constrains future work.

```markdown
### Postgres for all persistence
Decided 2026-09-14.
Why: transactional guarantees across the billing tables.
Rejected: MongoDB, which would have moved those guarantees into application code.
```

Without the rejected option written down, the next change reopens the question, because
nothing recorded that it was closed.

## Constraints

What this project will not do, and what it cannot do.

```
No ORM. Queries are written and reviewed.
No in-memory session state. Instances scale horizontally.
No new runtime dependency without discussion.
```

The section nobody writes and everybody needs. `design` reads it to rule out approaches
before proposing them.
