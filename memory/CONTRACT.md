# Memory contract

Every skill talks to memory through the operations below and never through a concrete
tool. `memory/adapters/<backend>.md` maps each operation to the calls the configured
backend actually exposes.

Changing backend means changing one adapter file. No skill changes.

## Source of truth

Which side is authoritative is `memory.documents`. It is a property of the repository,
recorded in its `.alfred/config.yaml`, not of the machine a run happens on: two people
working on the same repository produce the same files.

### keep

The default. Markdown files under version control are authoritative; memory holds a
searchable copy.

```
docs/changes/login-google/spec.md        authoritative
alfred/login-google/spec                 searchable copy
```

Memory can be wiped and rebuilt from the files at any time with `reindex`. The reverse is
not true, so no artifact exists only in memory.

### pointer

Memory holds the change artifacts; the repository holds their addresses.

```
alfred/login-google/spec                 authoritative
docs/changes/login-google/README.md      where to find it
```

`pointer` needs a backend that answers. `memory.required` reads as `true` under it whatever
it says, and the run stops before its first phase rather than producing a change whose
documents end up half in memory and half on disk. `backend: none` with `pointer` is a
configuration error, not a degraded mode.

What moves is the change directory, and only that. `docs/architecture.md`,
`docs/code_conventions.md` and everything under `paths.master_specs` stay files in both
modes: they are what the repository says about itself, they are read by people who are not
running Alfred, and the agent pointer files name them by path.

## The address file

Under `pointer`, `docs/changes/{change}/README.md` is written from
`templates/docs/addresses.md` and carries one row per artifact, appended by each phase as
it completes.

**The entry is stored before its address is written.** Between the two you have an entry
nothing names: unreachable, and harmless. In the other order you have an address naming
nothing, and the next phase fetches it, finds nothing, and fails in a way that reads like
memory loss rather than like a crash between two writes.

**Whatever commits the state file commits the address file.** They answer the same question
from two sides: state says which phase completed, the address file says where its output
went. State reading `spec: completed` beside an address file with no spec row leaves
`continue` resuming a phase whose input it cannot find.

## Reachability

The address file is the root. An entry that no address file names is unreachable, and no
phase acts on one. A change abandoned before its commit takes its address file with it, and
what it left in memory is residue rather than state.

`postmortem` is the exception, deliberately. Its purpose is to be found by a change that
did not exist when it was written, so `diagnose` searches for it instead of addressing it.
It is the one entry reachable without a pointer.

## Operations

### remember

```
remember(key, title, content, type, tags) -> id
```

Stores or replaces the entry at `key`. Called after a phase writes its document, so the
file and the copy stay in step. Repeated calls on the same `key` replace the entry rather
than appending a second one.

### recall

```
recall(query, type, limit) -> [{ id, key, title, preview, updated_at }]
```

Searches and returns candidates as previews. A preview is never enough to act on: it
exists to choose which entry to fetch.

### fetch

```
fetch(id) -> content
```

Returns the complete content of one entry. Always called before relying on anything
`recall` surfaced.

Searching and fetching are separate so a phase can scan ten candidates and load one,
rather than loading ten documents to read one.

### update

```
update(key, content) -> id
```

Replaces the content at a known `key` without searching for it.

### forget

```
forget(key)
```

Removes an entry. Used when a decision is reversed or an artifact is superseded, so stale
knowledge stops surfacing in `recall`.

### context

```
context(scope, limit) -> [{ id, key, title, preview, updated_at }]
```

Returns the most recent entries for a scope without a query. Used at the start of a phase
to recover what happened recently, and after a context reset to pick the thread back up.

### reindex

```
reindex(path) -> count
```

Rebuilds the searchable copy from the files under `path`. Run after cloning a repository,
after switching backend, or whenever memory and files may have diverged.

## Conflicts a backend cannot settle

A backend may answer `remember` with a conflict rather than a confirmation: what is being
stored disagrees with what is already there, and it will not pick between them.

That is the right answer. The wrong part is what happens next, when the conflict is raised
in a subagent that has no way to settle it — no tool for it, nobody to ask, and an empty
context that disappears when the phase ends. One measured run finished with six of them
open, each reported once, into nothing.

```
the phase records the conflict in its return, and continues
archive collects every conflict recorded during the change, in one list
the user settles them, or leaves them, in one pass
```

A phase never blocks on one. What the conflict is about is already written down twice, so
nothing is lost by deciding late, and a phase stopping to resolve a memory entry has
stopped the change for the index rather than for the work.

Never resolve one silently in either direction. An entry overwritten because the newer call
was newer loses whatever the earlier one knew, and the loss is invisible: both calls
succeeded.

A backend that raises conflicts on unrelated entries is reporting a tuning problem, not a
disagreement. Record it and say so — a channel that cries wolf gets ignored, and it is the
same channel that carries the real ones.

## Key naming

Keys are deterministic, so any phase can address an artifact without searching for it.

```
alfred/project/architecture
alfred/project/conventions
alfred/<change-name>/<artifact>
alfred/postmortem/<slug>
```

`<artifact>` is one of: `proposal`, `research`, `spec`, `design`, `tasks`, `diagnosis`,
`verify-report`, `review-report`.

```
alfred/login-google/spec
alfred/login-google/design
alfred/postmortem/payment-timeout-retry
```

## Types

| Type | Written by | Read by |
|---|---|---|
| `architecture` | `init`, `explore` | every phase |
| `conventions` | `init`, `explore` | `apply`, `review` |
| `proposal` | `refine` | `spec` |
| `research` | `research` | `spec`, `design` |
| `spec` | `spec` | `design`, `tasks`, `apply`, `verify`, `archive` |
| `design` | `design` | `tasks`, `apply`, `review` |
| `tasks` | `tasks` | `apply` |
| `diagnosis` | `diagnose` | `spec`, `design` |
| `verify-report` | `verify` | `archive` |
| `review-report` | `review` | `archive` |
| `postmortem` | `archive` | `diagnose` |

`postmortem` is the entry that makes a bug pay off twice: `diagnose` recalls it before
investigating, so a class of failure is diagnosed once rather than every time it appears.

## Passing artifacts to subagents

A subagent starts with an empty context. It receives keys, not content.

```
Task 3 of alfred/login-google/tasks
Spec:   alfred/login-google/spec
Design: alfred/login-google/design
```

The subagent recalls and fetches what its task actually needs. Pasting full documents into
the prompt would defeat the clean context it was given.

## Running without a backend

With `memory.required: false` and no backend configured, the pipeline still works because
the files exist regardless.

| Operation | Degraded behaviour |
|---|---|
| `remember`, `update` | no-op, the file is already written |
| `recall` | text search across `docs/` |
| `fetch` | read the file |
| `context` | most recently modified files under `docs/changes/` |
| `forget` | no-op |
| `reindex` | no-op |

The cost is tokens and search quality, never correctness.

## required

`memory.required` decides what happens when a backend *is* configured and does not answer.

| Value | A configured backend that does not answer |
|---|---|
| `false` | the run proceeds degraded; the orchestrator says so once, at the start |
| `true` | the run stops before the first phase, naming the backend |

`false` is the default, and it follows from the source of truth: the documents are
authoritative and memory is an index over them, so losing the index costs search quality
rather than correctness.

`memory.documents: pointer` reads as `true` regardless of what is written here, since
under it the documents are the thing that would be missing.

`true` is for where that trade stops holding. A `docs/` grown past what a text search can
rank turns every `recall` into a wider and wider read, and `diagnose` silently stops
finding the postmortems that are the reason they were written.

Neither value says anything about `backend: none`. Nothing is configured, so nothing is
unreachable, and the run proceeds on the table above.
