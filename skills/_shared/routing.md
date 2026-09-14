# Routing

Before any phase runs, the orchestrator chooses a route, states why, and waits for the
user to accept it.

## The question

> Does this change observable system behaviour?

Behaviour is what the system does as seen from outside: responses, stored data, messages a
user reads, side effects. Not how the code is arranged.

```
No   renaming a symbol, extracting a function, formatting,
     a dependency bump, a typo in a comment
     -> direct route

Yes  a new validation, a changed default, a new endpoint,
     an error message a user reads, a changed response shape
     -> pipeline
```

Counting files is the wrong test. A refactor touching thirty files changes nothing
observable; adding one validation to one file changes behaviour. The count is a proxy for
the question and gets both cases backwards.

## Routes

```
direct     apply -> verify -> review -> commit
pipeline   spec -> design -> tasks -> apply -> verify -> review -> archive
full       refine -> research -> spec -> ...
```

`verify` and `review` run on every route. The direct route skips planning, never
verification: a rename still has to compile, pass its tests and read well.

`archive` is skipped on the direct route because there is no delta to merge. Master specs
describe behaviour, and behaviour did not change.

Between `pipeline` and `full` sits nothing structural: `refine` and `research` are
`skippable`, so the difference is which of them the routing decision includes.

## Routing to refine

`refine` is included when the request is underspecified against the signals in
`skills/refine/SKILL.md`. Those signals are checkable, but they are not infallible, and a
wrongly triggered interview costs the user four rounds of questions they did not need.

So the decision is proposed, never applied silently.

```
This looks like a full run: no failure behaviour stated, more than one
component affected, and the auth strategy is undecided.

Start with the interview, or go straight to spec?
```

The user can always shorten the route. The orchestrator never lengthens it without
saying so.

## Stating the route

Every route decision names the signals it was based on. A route presented without its
reasons cannot be corrected, because the user cannot see which input was wrong.

## Changing route mid-run

A direct route that turns out to change behaviour stops and proposes the pipeline from
`spec`. Work already done is kept: the change exists, it simply needs a requirement
written for it.

The reverse does not happen. A pipeline run is never silently downgraded, because the
planning artifacts already exist and skipping them mid-run leaves a change half-specified.
