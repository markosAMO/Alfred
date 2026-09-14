---
backend: postgres
transport: sql
locality: remote
shared: true
---

# Remote database adapter

For a memory shared across a team. One database on a server, every developer's Alfred
writing to and reading from it.

The local adapters give one developer a memory of their own work. This one gives a team a
memory of each other's.

## Configuration

```yaml
memory:
  backend: postgres
  adapter: memory/adapters/postgres.md
  connection: env:ALFRED_MEMORY_URL
  required: false
```

The connection string is read from the environment, never written into a file that is
committed.

`required: false` still holds. A developer on a plane, or a database behind a VPN that is
down, degrades to `none.md` and keeps working.

## Schema

```sql
CREATE TABLE alfred_memory (
  id          BIGSERIAL PRIMARY KEY,
  project     TEXT NOT NULL,
  key         TEXT NOT NULL,
  title       TEXT NOT NULL,
  type        TEXT NOT NULL,
  content     TEXT NOT NULL,
  tags        TEXT[],
  author      TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  search      tsvector GENERATED ALWAYS AS
                (to_tsvector('simple', title || ' ' || content)) STORED,
  UNIQUE (project, key)
);

CREATE INDEX ON alfred_memory USING GIN (search);
CREATE INDEX ON alfred_memory (project, type, updated_at DESC);
```

`UNIQUE (project, key)` is what makes `remember` idempotent: the deterministic keys from the
contract mean a repeated write replaces rather than duplicates.

`project` isolates repositories sharing one database. Every query filters on it, so a
`recall` in one project never returns another project's decisions.

`author` records who wrote an entry. In a shared memory, knowing who decided something is
usually the next question after knowing what was decided.

## Operation mapping

| Contract | SQL |
|---|---|
| `remember` | `INSERT ... ON CONFLICT (project, key) DO UPDATE SET content, updated_at` |
| `recall(query)` | `SELECT id, key, title, left(content, 300), updated_at WHERE project = $1 AND search @@ plainto_tsquery($2) ORDER BY ts_rank DESC LIMIT $3` |
| `fetch(id)` | `SELECT content WHERE id = $1` |
| `update(key)` | `UPDATE ... WHERE project = $1 AND key = $2` |
| `forget(key)` | `DELETE WHERE project = $1 AND key = $2` |
| `context(scope)` | `SELECT ... WHERE project = $1 ORDER BY updated_at DESC LIMIT $2` |
| `reindex(path)` | walk the files, `remember` each, delete entries whose key no longer has a file |

`recall` returns truncated content, never full rows. The two-step retrieval in the contract
exists to keep a search from loading ten documents to read one, and that matters more over a
network than it does locally.

## Specifications still live in git

This adapter changes where the searchable copy lives. It does not change what is
authoritative.

```
docs/changes/login-google/spec.md    authoritative, in git, reviewed in a pull request
alfred_memory row                    searchable copy
```

A shared database is a reason to be stricter about this, not looser. Requirements that live
only in a database are not reviewed, not diffed, and not visible to someone reading the
repository.

## Concurrency

Two developers writing the same key is last-write-wins on `updated_at`. This is acceptable
because the file is the source of truth: the loser's file still exists and `reindex` restores
their copy.

## Switching

Moving between backends is a `reindex` from the files. There is no export or import, because
nothing exists in one backend that is not derivable from the repository.

```
memory.backend: engram -> postgres
alfred reindex
```
