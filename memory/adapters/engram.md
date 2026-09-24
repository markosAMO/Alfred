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
  engram mcp --tools=all
```

```jsonc
// OpenCode, in ~/.config/opencode/opencode.json
"mcp": {
  "engram": {
    "type": "local",
    "command": ["engram", "mcp", "--tools=all"],
    "enabled": true
  }
}
```

Use the absolute path to the binary where it is not on the agent's `PATH`.

`--tools=all` registers every tool Engram exposes — 23 at v2.0.0, across its `agent` (19)
and `admin` (4) profiles. Every one of them is schema an agent carries before it reads a
line, and that cost is paid deliberately.

An earlier version of this file named a subset, and `scripts/generate_agents.py` narrowed it
again per phase. Both were wrong in the same way: **a phase cannot tell a tool it was not
given from a backend that cannot do the thing.** Both read as absence from inside the phase,
so a missing tool degrades into a capability the run silently does without. Observed: an
`archive` that hit `judgment_required` on every save, had no `mem_judge`, and recorded six
unsettled conflicts rather than settling them.

A subset is also a copy of the backend's surface that nothing keeps in step. Engram grows a
tool and the list does not, and the gap only ever shows up as a phase quietly doing less.

Which operations a phase *calls* is `memory/CONTRACT.md`'s business, and it is still narrow —
`What this adapter does not call` below says so. What is *reachable* is this list, and it is
everything.

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

**A `mem_save` without `topic_key` is not a `remember`.** The field is optional to Engram,
which accepts the call, stores the entry, returns an id and *suggests* a key it does not
apply. Nothing fails. What you get is an entry that:

```
cannot be fetched by key      alfred/{change}/verify-report resolves to nothing
does not replace on rewrite   the next remember() adds a second copy
is reachable only by search   which the contract forbids relying on
```

Observed in a full run: seven of eight artifacts carried their key and `verify-report` did
not. The phase reported success, `archive` confirmed "every document is present" — by
searching, which found it — and the defect survived both.

So the key is passed on every call, and it is checked: the `mem_save` result echoes the
entry it wrote, and a result whose `topic_key` is absent or different from the key asked for
is a failed `remember`, reported through `notify`, not a warning to skip past.

`type` is passed through from the contract's vocabulary unchanged — `spec`, `design`,
`tasks`, `verify-report`, `review-report`, `proposal`, `research`, `diagnosis`,
`postmortem`, `architecture`, `conventions`. Engram accepts each of them; its own native
types (`decision`, `pattern`, `discovery`, `manual`) are what a human saving by hand uses,
and a phase that reaches for one has thrown away the distinction `recall(query, type)`
depends on. In the same run, **none of the eight artifacts carried a contract type** — a
`verify-report` was stored as `decision` and a `review-report` as `manual`, so filtering a
recall by type would have returned neither.

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

`mem_delete(id)` removes an observation, and Engram does expose it. This adapter still does
not call it.

A superseded entry is rewritten through `update`, its content replaced by a note naming what
replaced it.

```
superseded by alfred/{change}/spec, 2026-09-14
```

The entry stops asserting something false, which is what `forget` is for, and the fact that
it was retired survives. A hole where an entry used to be answers no question; `recall`
returns nothing either way, and nobody can tell a decision that was reversed from one that
was never recorded.

This is a choice, not a limitation. An earlier version of this file claimed Engram had no
deletion through MCP, which was wrong. Where a caller genuinely needs the row gone —
material stored by mistake, something that should never have been written down — `mem_delete`
is the call, and it is the one case this adapter makes it.

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
  engram mcp --tools=all
```

`mem_judge` absent from that list is not a backend that cannot settle conflicts. It is a
tool that was never offered, and it reads identically from inside the phase — which is the
same trap as a memory tool that is deferred and not yet loaded, below.

Conflicts raised against entries with a near-zero similarity score are a threshold that
wants tuning rather than a disagreement worth a user's attention. Record the observation
with the conflict; a channel that cries wolf is ignored, and it carries the real ones too.

## Reading what a conflict points at

A conflict is reported as a `sync_id`:

```
conflict: contested by #obs-4878b291a98ae4cf (pending)
```

`mem_get_observation` does not accept one. Passed a `sync_id` it answers `id is required`,
and the only identifier it takes is the numeric `id`. So the identifier the backend hands a
phase when it raises a conflict is not the identifier the phase needs to read it.

The two are paired in the `results` array of a `mem_search`, and that is the way across:

```
mem_search(...)  ->  results[] carrying both  { id: 75, sync_id: "obs-4878b291a98ae4cf" }
mem_get_observation(id: 75)
```

So a phase resolves a contested entry by searching for it and matching on `sync_id`, never
by passing the `sync_id` it was given. A phase that passes it straight through gets an error
that reads like a malformed call rather than like a missing translation, and the usual
response — record the conflict unread and move on — is how twenty conflicts accumulate with
nobody ever having seen what either side said.

Where the search does not surface the entry, the conflict is reported with its `sync_id`
unresolved and named as such. `engram conflicts show <id>` reads it from a terminal, which
is a person's tool and not this pipeline's.

## Recall is not reliably semantic

Observed in one run: `mem_search` matched an entry on its exact `topic_key`, and a
natural-language query carrying the same distinctive terms — the area name, the change name,
the word "findings" — returned nothing at all.

Two consequences for how phases search.

**An artifact is recalled by its key, never by describing it.** The keys are deterministic
precisely so this never depends on a query working, per `Key naming` in the contract. A
phase that searches for "the spec for the login change" is relying on the weakest thing the
backend does.

**A `recall` that returns nothing is not evidence that nothing is there.** It is the one
result that must never be reported as "no prior work exists". Where a phase needs to know
that, it asks by key and treats an empty answer as an unwritten artifact; where it is
genuinely exploring, an empty result is reported as an empty search.

This is the same trap as a missing tool, one level down: the failure returns success, and it
looks exactly like the world being empty.

## Sharing memory with the repository

Engram is local to the machine. `engram sync` exports this project's entries to `.engram/`
inside the repository, and that directory is committed.

```bash
engram sync            # after a run, export the project's entries to .engram/
engram sync --import   # after cloning, load them
```

```gitignore
.engram/engram.db      # the local database, never committed
.engram/cloud.json     # local credentials, never committed
```

Everything else under `.engram/` is committed on purpose: `manifest.json` and the gzipped
chunks under `chunks/`, which are the entries themselves. They are compact — several hundred
entries compress to a few hundred kilobytes, which is less than one change costs in prose.

This is what makes `memory.documents: ephemeral` safe for more than one person. The working
documents are removed at close, and the two copies that remain are the commit and memory. A
memory that never leaves one laptop makes the second copy worthless to everyone else, and
the first person to clone the repository gets the record and nothing behind it.

Run `sync` after a change closes, and commit `.engram/` with it. `init` writes the two
ignore lines when the backend is `engram`.

## Project naming

Engram groups entries by project, derived from the git remote and lowercased. Two things
follow, and both bite Alfred specifically.

**The project is resolved from the session the MCP server runs in, not from the working
directory of the agent that calls it.** A subagent dispatched with its own working directory
still writes to the project the server resolved, and it is not told otherwise: the call
succeeds and reports the project it used, which nobody reads.

Observed directly. A change run entirely inside one repository, by subagents whose working
directory was that repository, indexed all five of its artifacts against a *different*
project — the one the server had resolved from the session that launched them.

Two consequences, both of which look like success:

```
a change run against another checkout      its documents land in the launching project
a repository with no remote                falls back to a directory name
```

A worktree under `git.worktrees.root` is the case this is safe for, and the reason the MCP
server is registered machine-wide: the worktree shares the remote, and the session that runs
the change is the change's own.

The case it is not safe for is a phase dispatched against a repository the session did not
start in. `archive` records the keys it actually wrote in the record, which is what makes
the mistake findable afterwards; `mem_merge_projects` merges what diverged, and
`alfred doctor` reports a repository whose remote does not match the backend's project.

## Prompt capture

`capture_prompt: false` is set on every pipeline artifact, per the mapping above. A phase
document is generated output, not something the user said, and recording it as a prompt
pollutes the history that `context` returns.

`mem_save_prompt` is the other half and belongs to whatever observes the user directly —
the orchestrator, or a hook. It records the real request before any derived `mem_save`, so
Engram can attach and deduplicate it. A phase never calls it: a phase did not see the user.

## Session lifecycle belongs to the orchestrator

`mem_session_start` and `mem_session_end` bracket a run. The orchestrator calls them,
because it is the participant that lives for the whole one; a phase that called them would
open and close a session per phase and group nothing.

Unbracketed, saves attach to whatever session Engram last had open. Observed: a seven-phase
run whose every artifact attached to a session opened two days earlier, so the run is not a
unit anything can retrieve — `context(scope)` returns the stale session's contents and calls
them recent.

`mem_save_prompt` is the orchestrator's too, and for the same reason: it records the user's
actual request before any derived `mem_save`, and the orchestrator is the only participant
that saw the user. A phase never calls it.

## What this adapter does not call

`mem_session_summary` is for a top-level agent, never a subagent. A phase runs in a session
that exists for one phase, and a summary written from inside it describes that fragment as
though it were the run.

`mem_capture_passive`, `mem_timeline`, `mem_stats`, `mem_doctor`, `mem_list_projects`,
`mem_pin` and `mem_unpin` are inspection and housekeeping tools for a person at a terminal
or for `alfred doctor`. Every agent is *given* them, per the tool list above — a phase must
never be unable to tell a missing tool from a backend that cannot — but no phase in the
pipeline has a reason to call one.

The distinction is the point: **what is reachable is the whole backend; what a phase calls
is this contract.** Narrowing the first to match the second is what produced an `archive`
that could not settle a conflict it had correctly detected.

## Lifecycle metadata

Where `mem_review` is available, entries marked `needs_review` are treated as stale context
rather than as facts, and are verified against current evidence before being relied on.
Entries are never marked reviewed automatically.

Where it is unavailable, the run continues without it. Lifecycle metadata improves recall
quality; it is not required for correctness.

## Failure

Any failed call is reported through `notify` as a warning and the run continues on the
degraded behaviour in `none.md`. The artifacts are files, so nothing is lost.
