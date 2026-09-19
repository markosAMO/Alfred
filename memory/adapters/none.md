---
backend: none
transport: filesystem
locality: local
---

# No-backend adapter

The default when no memory backend is configured, and the fallback any other adapter
degrades to.

The pipeline works without a memory backend because the artifacts are files. Memory makes
them searchable; it does not hold them.

## Operation mapping

| Contract | Behaviour |
|---|---|
| `remember` | no-op, the phase already wrote the file |
| `update` | no-op, same reason |
| `recall(query)` | text search across `docs/`, ranked by filename and heading matches |
| `fetch(id)` | read the file, where the identifier is its path |
| `context(scope)` | most recently modified files under `docs/changes/` |
| `forget` | no-op |
| `reindex` | no-op |

## Key resolution

Contract keys map onto paths.

```
alfred/project/architecture     docs/architecture.md
alfred/{change}/spec            docs/changes/{change}/spec.md
alfred/postmortem/{slug}        docs/changes/*/diagnosis.md matching the slug
```

## What is lost

Recall is lexical, so a postmortem about a payment timeout is not found by searching for a
notification timeout. `diagnose` still works; it just investigates from scratch more often.

Nothing else changes. No phase fails, no artifact is missing, and switching to a real
backend later is a `reindex` away because the files were the source of truth the whole time.

## Not a mode for pointer

`memory.documents: pointer` puts the change artifacts in memory and keeps only their
addresses in the repository. With no backend there is nowhere to put them, so the
combination is a configuration error and `init` and `doctor` report it as one. This adapter
is reachable only under `keep`.

## What is not lost

Pipeline state. State lives in `.alfred/state/`, not in memory, per
`skills/_shared/state-contract.md`. `continue` works identically with no backend.
