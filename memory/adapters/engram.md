---
backend: engram
transport: mcp
locality: local
---

# Engram adapter

Maps `memory/CONTRACT.md` onto Engram's MCP tools. A single Go binary over SQLite with
full-text search, running on the same machine.

## Requirements

Engram installed and exposed to the agent as an MCP server. `memory.required: false` means
a missing binary degrades to `none.md` rather than failing the run.

## Operation mapping

| Contract | Engram |
|---|---|
| `remember(key, title, content, type, tags)` | `mem_save(title: key, topic_key: key, content, type, project)` |
| `recall(query, type, limit)` | `mem_search(query, project, limit)` |
| `fetch(id)` | `mem_get_observation(id)` |
| `update(key, content)` | `mem_update` when available, otherwise `mem_save` with the same `topic_key` |
| `context(scope, limit)` | `mem_context(project, limit)` |
| `forget(key)` | see below |
| `reindex(path)` | see below |

`topic_key` carries the contract's `key` unchanged. Reusing it is what makes a repeated
`remember` replace an entry instead of accumulating competing copies of the same artifact.

`capture_prompt: false` is set where the schema exposes it, so pipeline artifacts are not
recorded as conversational prompts. An older schema that rejects the field has it omitted
rather than failing the call.

## Two-step retrieval

`mem_search` returns previews and identifiers. `mem_get_observation` returns content. Both
are always used in that order, never a search result acted on directly, per the contract.

## forget

Engram does not expose deletion through MCP. A superseded entry is rewritten through
`update` with its content replaced by a supersession note naming what replaced it.

```
superseded by alfred/{change}/spec, 2026-09-14
```

The entry stops asserting something false, which is what `forget` is for. A record that an
entry was retired is more useful in a memory than a hole where it used to be.

## reindex

Engram has no bulk import. Reindexing walks `docs/` and calls `remember` for each artifact,
deriving each key from its path per the contract's naming rules. Existing entries are
replaced by `topic_key`, so the operation is safe to repeat.

## Lifecycle metadata

Where `mem_review` is available, entries marked `needs_review` are treated as stale context
rather than as facts, and are verified against current evidence before being relied on.
Entries are never marked reviewed automatically.

Where it is unavailable, the run continues without it. Lifecycle metadata improves recall
quality; it is not required for correctness.

## Failure

Any failed call is reported through `notify` as a warning and the run continues on the
degraded behaviour in `none.md`. The artifacts are files, so nothing is lost.
