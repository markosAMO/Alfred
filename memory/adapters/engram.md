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
  engram mcp --tools=mem_search,mem_get_observation,mem_save,mem_update,mem_context,mem_judge
```

```jsonc
// OpenCode, in ~/.config/opencode/opencode.json
"mcp": {
  "engram": {
    "type": "local",
    "command": ["engram", "mcp", "--tools=mem_search,mem_get_observation,mem_save,mem_update,mem_context,mem_judge"],
    "enabled": true
  }
}
```

Use the absolute path to the binary where it is not on the agent's `PATH`. The tool list is
the operations this adapter uses; Engram exposes more, and every one declared is schema an
agent carries before it reads a line. `mem_judge` earns its place because without it a
conflict raised in a subagent has nobody to settle it; see `Conflicts` below.

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

## What fetch returns

`mem_get_observation` returns the stored document wrapped, not bare: a header line
`#<id> [<type>] <title>` before it, and a metadata block after it.

```
Session: ...
Project: ...
Scope: ...
Topic: alfred/project/architecture
Duplicates: 1
Revisions: 1
Created: ...
```

A check that compares a fetched entry against its file — `reindex` makes one before
deleting anything under `memory.documents: pointer` — asks whether the document is present
intact inside what came back, never whether the two are equal. Equality fails on the
wrapper alone, and would refuse every migration that was in fact correct.

`Duplicates` and `Revisions` are how a repeated `remember` on the same `topic_key` reports
itself: one entry, revised, rather than a second copy.

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

## Conflicts

`mem_save` can answer `judgment_required: true` with the entries it disagrees with. Engram
exposes a tool that settles it, and where it is available to the phase, the phase settles
the conflict and continues.

Where it is not — a subagent whose tool list does not carry it — the phase records the
conflict in its return and continues, per the contract. `archive` collects them into one
list for the user. Nothing blocks and nothing is overwritten.

The tool list a session declares is what decides which of those two happens, so a
subagent that will write memory is granted the same tools the session has:

```bash
claude mcp add engram --scope user -- \
  engram mcp --tools=mem_search,mem_get_observation,mem_save,mem_update,mem_context,mem_judge
```

`mem_judge` absent from that list is not a backend that cannot settle conflicts. It is a
tool that was never offered, and it reads identically from inside the phase — which is the
same trap as a memory tool that is deferred and not yet loaded, below.

Conflicts raised against entries with a near-zero similarity score are a threshold that
wants tuning rather than a disagreement worth a user's attention. Record the observation
with the conflict; a channel that cries wolf is ignored, and it carries the real ones too.

## Lifecycle metadata

Where `mem_review` is available, entries marked `needs_review` are treated as stale context
rather than as facts, and are verified against current evidence before being relied on.
Entries are never marked reviewed automatically.

Where it is unavailable, the run continues without it. Lifecycle metadata improves recall
quality; it is not required for correctness.

## Failure

Any failed call is reported through `notify` as a warning and the run continues on the
degraded behaviour in `none.md`. The artifacts are files, so nothing is lost.
