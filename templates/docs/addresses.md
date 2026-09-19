---
change: {change}
phase: addresses
---

# {change}

One line on what this change did, in the present tense, for a reader who arrives from a
master specification and wants to know whether to open anything else.

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
| Result | `completed` or `failed`, with the reason |
| Commit | |
| Specifications changed | paths under `paths.master_specs` |
| Postmortem | `alfred/postmortem/{slug}`, for a bug |

## Recovering the documents

```
fetch <key>
```

Or, for the whole change at once, `reindex` after restoring the backend from its own
backup. These documents are not in git: this repository is configured
`memory.documents: pointer`.
