---
change: {change}
phase: addresses
---

# {change} — addresses

Where this change's documents are stored. **What the change did is in `record.md`, beside
this file.** This one exists to be addressable, not to be read: it is the index that makes
the entries reachable, per `Reachability` in `memory/CONTRACT.md`.

Two files rather than one, under `pointer`, because they answer different questions. The
record is prose for a person arriving from a master specification; this is a key per
artifact for a phase that has to fetch one. Merging them would put machine addresses in the
middle of the document somebody actually reads.

## Where the documents are

Backend `{backend}`. Each key is addressable directly; nothing here needs to be searched
for.

| Artifact | Key | Written by | At |
|---|---|---|---|
| | `alfred/{change}/...` | | |

A row is appended by each phase as it completes, after its entry is stored. A row with no
entry behind it is a defect, not a gap: see `memory/CONTRACT.md`.

## Outcome

| | |
|---|---|
| Record | `record.md` — the outcome, the decisions, what was required |
| Postmortem | `alfred/postmortem/{slug}`, for a bug |

The result, the commit and the specifications that changed are in the record and are not
repeated here. Two places carrying the same outcome go out of step at the first correction,
and nothing says which one is current.

## Recovering the documents

```
fetch <key>
```

Or, for the whole change at once, `reindex` after restoring the backend from its own
backup. These documents are not in git: this repository is configured
`memory.documents: pointer`.
