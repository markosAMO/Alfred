---
name: tasks
mode: auto
skippable: false
reads: [spec, design, conventions]
writes: [task_list]
document: docs/changes/{change}/tasks.md
next: [apply]
---

# tasks

Break the design into units a subagent can complete alone, in an order that respects what
depends on what.

Each task is executed by a subagent with an empty context. A task that cannot be understood
from its own description plus the documents it names is a task that will be done wrong.

## Sizing

A task is one coherent piece of work with a verifiable end.

```
too large   "implement authentication"
            no single point where it is done, no way to review it
too small   "add an import statement"
            the dispatch costs more than the work
right       "add the OAuth callback endpoint with its success and failure tests"
```

The test is whether completion can be stated without "partly". A task ends with something
that compiles, passes its tests, and can be read on its own.

## Scenario coverage

Every scenario in the specification is claimed by exactly one task.

```markdown
- [ ] 3. Add the OAuth callback endpoint
      covers: [successful sign-in, consent denied]
      depends_on: [2]
      files: app/controllers/auth_controller.rb
```

A scenario claimed by no task will not be implemented, and `verify` will fail on it after
the work is done rather than before it started. Coverage is checked here, while the cost of
fixing it is one line.

A scenario claimed by two tasks produces two implementations of the same behaviour.

## Dependencies

A task depends on another when it needs something that task creates: an interface, a
schema, a module. `design` names those under `Interfaces`, and that section is what this
phase reads to order the work.

```yaml
depends_on: [2]
```

Dependencies are declared, never implied by order in the list. A subagent is dispatched
only once its dependencies are `completed`, per `skills/_shared/subagent-protocol.md`, and
tasks with no dependency between them may run together.

Across repositories, the same declaration is made at repository granularity. See
`skills/_shared/workspace-protocol.md`.

## Tests are not separate tasks

A task includes the tests for the scenarios it claims. Splitting them produces a task that
is "done" with no evidence, and a second task nobody prioritises.

## Tracker

When a tracker is configured, each task is mirrored as a card and its identifier written
back into the document.

```markdown
- [ ] 3. Add the OAuth callback endpoint
      external_id: PROJ-1487
```

The file is authoritative. A tracker that fails is reported as a warning and the phase
continues, per `tracker/CONTRACT.md`.

## What it writes

```markdown
## Tasks
## Dependency order
## Scenario coverage
```

`Scenario coverage` maps every scenario in the spec to the task that claims it, so the gap
is visible rather than inferred.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the document, index it under
`alfred/{change}/tasks`, update state with the task list and dependency graph, notify
`phase_completed`.

Report what the user checks:

```
8 tasks, 7 scenarios covered, 3 can run in parallel
```
