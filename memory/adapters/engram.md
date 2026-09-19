---
backend: engram
transport: mcp
locality: local
---

# Engram adapter

Maps `memory/CONTRACT.md` onto Engram's MCP tools. A single Go binary over SQLite with
full-text search, running on the same machine.

## Requirements

Engram installed, and registered with the agent as an MCP server. The two are separate
steps and the second is the one that gets forgotten: the binary can be installed and the
config can name `backend: engram` while the agent has never been told the server exists,
in which case the tools this adapter maps onto are simply absent. Under
`memory.required: false` that degrades to `none.md` in silence.

The server must be named `engram`, since the tool names the agents declare are built from
it.

```bash
# Claude Code, machine-wide so worktrees outside the repository are covered too
claude mcp add engram --scope user -- \
  engram mcp --tools=mem_search,mem_get_observation,mem_save,mem_update,mem_context
```

```jsonc
// OpenCode, in ~/.config/opencode/opencode.json
"mcp": {
  "engram": {
    "type": "local",
    "command": ["engram", "mcp", "--tools=mem_search,mem_get_observation,mem_save,mem_update,mem_context"],
    "enabled": true
  }
}
```

Use the absolute path to the binary where it is not on the agent's `PATH`. The tool list is
the five operations this adapter uses; Engram exposes more, and every one declared is
schema an agent carries before it reads a line.

Tool names differ per agent: `mcp__engram__mem_search` in Claude Code, `engram_mem_search`
in OpenCode. `models.profile` carries the Claude Code form as `memory_tool_prefix`.

A run against a repository whose `docs/` predates the registration starts with a memory
holding nothing, since every `remember()` until then was a no-op. `reindex` builds it from
the files.

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
