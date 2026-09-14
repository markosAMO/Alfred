---
document: architecture
scope: default
offered_by: init
---

# Architecture

The starting point `init` offers for a new project. Every section is a question until it is
answered, and the answers become the constraints every later phase reads.

Answer what is decided. Leave the rest, and `design` will raise it the first time a change
depends on it.

## Stack

> Which languages, frameworks and runtimes, at which versions?
> What runs where: processes, services, scheduled work?

## Layers and boundaries

> What layers does this project have?
> Which may call which, and in what direction?
> What must never cross: does a controller reach a repository, does a domain object know
> about HTTP?

Write the direction, not only the names. `review` enforces what is written here, so a
boundary left unwritten cannot be enforced.

## Persistence

> Where does state live: database, cache, queue, files?
> When two of them disagree, which one is right?
> What is kept, and for how long?

## External services

> What does this project consume, and for what?
> What happens when one does not respond: fail, retry, degrade, queue?

The failure behaviour is the part that gets skipped and the part that matters at three in
the morning.

## Cross-cutting

> How is authentication and authorisation done?
> How are errors raised, logged and surfaced?
> Where does configuration come from, and where do secrets come from?
> How is background and scheduled work run?

Answered once here so no feature invents its own version.

## Decisions

> What has already been decided that future work must respect?
> For each: why, and what was rejected?

```markdown
### {decision}
Decided {date}.
Why: {reason}
Rejected: {alternative, and what it would have cost}
```

The rejected option is the half that stops the question being reopened in six months.

## Constraints

> What will this project not do?
> What can it not do?

```
No ORM. Queries are written and reviewed.
No in-memory session state. Instances scale horizontally.
No new runtime dependency without discussion.
```

The section nobody writes and everybody needs. `design` reads it to rule out approaches
before proposing them, which is cheaper than rejecting them in review.
