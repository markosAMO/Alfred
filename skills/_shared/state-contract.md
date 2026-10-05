# Pipeline state contract

State lives in a file, never in memory search. `continue` needs an exact answer to "which
phase is next", and a semantic search that misses would skip or repeat a phase.

One file per change, under `paths.pipeline_state`.

```
.alfred/state/login-google.yaml
```

## Shape

```yaml
change: login-google
title: Login with Google
type: feature
workflow: sdd
entry_point: refine
origin_channel: openclaw
created_at: 2026-09-13T04:20:00Z
updated_at: 2026-09-13T06:05:00Z

current_phase: design
status: waiting_for_input

phases:
  refine:   { status: completed, at: 2026-09-13T04:40:00Z }
  research: { status: skipped }
  spec:     { status: completed, at: 2026-09-13T05:10:00Z }
  design:   { status: in_progress }
  tasks:    { status: pending }
  apply:    { status: pending }
  verify:   { status: pending }
  review:   { status: pending }
  archive:  { status: pending }

tasks:
  completed: []
  pending: []
  blocked: []

parent: null
repos: []

worktree: null
branch: null
base: null
main_checkout: null
```

## Fields

| Field | Purpose |
|---|---|
| `type` | which of the workflow's entry points applies |
| `workflow` | the workflow the change runs under; its routes, groups and closing phase are the ones in force |
| `entry_point` | the phase the change entered at, one the workflow declares |
| `origin_channel` | where the run started, so blocking questions return there; for a change in its own session it names the coordinator |
| `current_phase` | what `continue` resumes |
| `status` | `running`, `waiting_for_input`, `waiting_for_confirmation`, `completed`, `failed` |
| `parent` | `workspace:<change>` when this repository is part of a multi-repository change |
| `repos` | at workspace level, the repositories this change was distributed to |
| `worktree` | the checkout this change runs in, when it runs in its own; `null` otherwise |
| `branch` | the branch that worktree is on |
| `base` | the ref the branch was created from |
| `main_checkout` | the repository the worktree belongs to, where `.alfred/config.yaml` lives |

The four worktree fields are copied from what `worktree.sh open` printed, and are what the
closing phase needs to close the worktree once the change is committed. See
`skills/_shared/worktree-protocol.md`.

## The workflow a change runs under

`workflow` is written by the first phase of the change, from what the orchestrator passed
it, and never changes afterwards. No phase derives it: the orchestrator is the only
participant that knows which command the run started from, and a phase guessing would pick
whichever workflow looks like the one it belongs to.

It is in the file because a state file is read by a run that did not start the change.
`continue` resumes under that workflow's rules and routes and dispatches the phase that
workflow's route says is next, which it can only do by being told which workflow that is.
Without the field, a change started under one recipe would be resumed under whichever one
the resuming command happened to carry — a substitution nothing would report, because
every phase name involved would still resolve.

A workflow named here and available in neither the repository nor the machine stops
`continue`, which reports which workflow the change needs and resumes nothing. The change
is not lost: installing or restoring that workflow makes it resumable again, and that is a
better outcome than finishing it under rules nobody chose for it.

## Phase status

| Status | Meaning |
|---|---|
| `pending` | not started |
| `in_progress` | started, document not written yet |
| `completed` | document written and indexed |
| `skipped` | declared skippable and not run |
| `failed` | attempted and could not finish; carries `reason` |

## Rules

State is updated only after the phase document exists on disk. A phase marked `completed`
whose document is missing makes `continue` skip work that was never done.

`failed` carries the reason and never advances `current_phase`, so `continue` retries the
phase that failed rather than moving past it.

State is committed with the change. A collaborator who clones the repository mid-change
can run `continue` and pick up exactly where it stopped. What is committed at the end of a
change is its removal rather than its contents, per `Lifecycle` below.

## Lifecycle

State exists so `continue` can resume a change. A change that closed has nothing to resume,
and its state file stops describing anything: every phase reads `completed`, and the row
`continue` would act on is the absence of a next one.

The phase the workflow declares as its closing one removes it, under
`artifacts.state_on_completion: delete`. Which phase that is differs between workflows and
changes nothing here, per `skills/_shared/workflow-protocol.md`.

```
the change closed        the closing phase deletes .alfred/state/{change}.yaml
anything else            the file stays
```

**Anything else** is the whole point of the setting, so it is worth naming rather than
implying:

```
a phase failed                          state stays, carrying the reason
a phase before the closing one blocked  closing never ran, so state stays
the change was abandoned                nothing closed it, so state stays
the closing phase closed open work      state stays, and what it wrote says why
```

The last one is the one that looks wrong and is not. A change that did not finish can be
closed, when the user asks for it explicitly. What the closing phase writes then reports
what was left open, and the state file stays because open work is work somebody may come
back to. Deleting it would leave a record saying "three tasks unfinished" and nothing able
to resume them.

Deletion is staged with the commit that closes the change, so the file disappears from the
repository and from the working tree together. A state file deleted on disk but still
tracked comes back on the next checkout, and `continue` finds a change that finished months
ago.

`artifacts.state_on_completion: keep` turns this off and is for a repository that wants the
state files as a ledger of what ran. The cost is that `.alfred/state/` grows without bound
and `alfred status` lists changes nobody is working on.

## What deletion is not

Removing the state file does not remove the change. The record under
`paths.change_records` is what says the change happened, the delta specification is what
says what it required, and the master specifications carry the behaviour. State is
bookkeeping for a run in progress, and it is the only thing here that is disposable.
