# Memory contract

Every skill talks to memory through the operations below and never through a concrete
tool. `memory/adapters/<backend>.md` maps each operation to the calls the configured
backend actually exposes.

Changing backend means changing one adapter file. No skill changes.

## Source of truth

Markdown files under version control are authoritative. Memory holds a searchable copy.

```
docs/changes/login-google/spec.md        authoritative
alfred/login-google/spec                 searchable copy
```

Memory can be wiped and rebuilt from the files at any time. The reverse is not true, so
no artifact exists only in memory.

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

`true` is for where that trade stops holding. A `docs/` grown past what a text search can
rank turns every `recall` into a wider and wider read, and `diagnose` silently stops
finding the postmortems that are the reason they were written.

Neither value says anything about `backend: none`. Nothing is configured, so nothing is
unreachable, and the run proceeds on the table above.
