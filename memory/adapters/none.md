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

## Ephemeral against no backend

`memory.documents: ephemeral` is reachable here, and it is the one combination that
genuinely discards something. The working documents are removed when `archive` closes the
change, and with no backend there is no second copy: they are gone.

That is a supported configuration, not an accident. What a later reader needs — what
changed, why, how it was decided, what it required and how it ended — is lifted into the
record before anything is deleted, and the delta specification and the master
specifications stay as files. What is discarded is the reasoning-in-progress that produced
them.

It still deserves to be said out loud rather than discovered. `init` says it when the mode
is chosen against `backend: none`, and `alfred doctor` reports the pair as a condition worth
knowing about rather than as an error.

## Not a mode for pointer

`memory.documents: pointer` puts the change artifacts in memory and keeps only their
addresses in the repository. With no backend there is nowhere to put them, so the
combination is a configuration error and `init` and `doctor` report it as one. This adapter
is reachable under `keep` and `ephemeral`, never under `pointer`.

## What is not lost

Pipeline state. State lives in `.alfred/state/`, not in memory, per
`skills/_shared/state-contract.md`. `continue` works identically with no backend.
