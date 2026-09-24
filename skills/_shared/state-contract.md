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
| `type` | `feature` or `bug`; decides which entry point applies |
| `entry_point` | `refine` or `diagnose` |
| `origin_channel` | where the run started, so blocking questions return there; for a change in its own session it names the coordinator |
| `current_phase` | what `continue` resumes |
| `status` | `running`, `waiting_for_input`, `waiting_for_confirmation`, `completed`, `failed` |
| `parent` | `workspace:<change>` when this repository is part of a multi-repository change |
| `repos` | at workspace level, the repositories this change was distributed to |
| `worktree` | the checkout this change runs in, when it runs in its own; `null` otherwise |
| `branch` | the branch that worktree is on |
| `base` | the ref the branch was created from |
| `main_checkout` | the repository the worktree belongs to, where `.alfred/config.yaml` lives |

The four worktree fields are copied from what `worktree.sh open` printed, and are what
`archive` needs to close the worktree once the change is committed. See
`skills/_shared/worktree-protocol.md`.

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

`archive` removes it, under `artifacts.state_on_completion: delete`.

```
the change closed        archive deletes .alfred/state/{change}.yaml
anything else            the file stays
```

**Anything else** is the whole point of the setting, so it is worth naming rather than
implying:

```
a phase failed                          state stays, carrying the reason
verify failed or review blocked         archive never ran, so state stays
the change was abandoned                nothing closed it, so state stays
archive closed incomplete work          state stays, and the record says why
```

The last one is the one that looks wrong and is not. `archive` can close a change that did
not finish, when the user asks for it explicitly. The record then reports what was left
open, per its Result section, and the state file stays because open work is work somebody
may come back to. Deleting it would leave the record saying "three tasks unfinished" and
nothing able to resume them.

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
