# Task tracker contract

`tasks` always writes `tasks.md`. When a tracker is configured, it also creates the cards.

The file is authoritative; the tracker is a mirror. A tracker that is down, rate-limited or
misconfigured degrades the run to the file alone and never stops it.

`tracker/adapters/<name>.md` maps each operation to what that tracker exposes. The default
adapter is `none`, and a project with no tracker uses the pipeline unchanged.

## Operations

### fetch_task

```
fetch_task(external_id) -> { title, description, type, labels, parent }
```

Pulls an existing card so it can enter the pipeline as raw input. What comes back is a
request, not a task list: the orchestrator decides which phases it needs.

```
card fetched -> vague description   -> refine
             -> clear requirement   -> spec
             -> defect report       -> diagnose
             -> epic                -> several changes, several child cards
```

A card is never executed directly. Work that skips `spec` has no scenarios, so `verify`
has nothing to check and `archive` has nothing to merge.

### create_subtask

```
create_subtask(parent_external_id, title, description) -> external_id
```

Creates a child card under the one that entered the pipeline. Used when analysis shows the
original card covers more than one change.

### create_task

```
create_task(change, task_id, title, description, labels) -> external_id
```

Creates one card and returns the tracker's identifier, which is written back into
`tasks.md` so the file and the board can be matched later.

### update_task

```
update_task(external_id, status, note)
```

Reflects a task moving to `in_progress`, `completed`, `failed` or `blocked`.

### close_task

```
close_task(external_id, resolution)
```

Closes a card once its task passed `verify` and `review`.

### link_tasks

```
link_tasks(external_id, depends_on_external_id)
```

Records a dependency declared by `tasks` on the board, so the ordering Alfred enforces is
visible to people who only look at the tracker.

## Identifiers

`tasks.md` carries both identifiers for every task.

```markdown
- [ ] 3. Add the OAuth callback endpoint
      depends_on: [2]
      external_id: PROJ-1487
```

The local id orders the work. The external id exists only to keep the board in step.
A task with no `external_id` is a task Alfred could not mirror, not a task that does not
exist.

## Direction of truth

The board is never polled. A card enters the pipeline only when it is handed to the
orchestrator explicitly, through `fetch_task`, and enters as input rather than as work to
execute.

Everything after that flows outward: Alfred writes cards, updates and closes them. The
file remains authoritative, so a board that is unreachable costs visibility, never
progress.

## Failure

A tracker call that fails is recorded in state and reported through `notify` as a warning.
The phase continues. Cards can be reconciled later from `tasks.md`, which holds everything
needed to recreate them.
