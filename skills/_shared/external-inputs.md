# External inputs

Material that arrives as a reference — a tracker card, a URL, a document in another system —
is read once, written to the repository as text, and never fetched again.

Every phase after that reads the text. No phase and no subagent reaches the network for
material that was already brought in.

## Why once

A subagent that fetches a Jira card needs credentials, needs the network, and gets whatever
the card says at that moment. The same task run twice produces different work, and the run
cannot be reproduced later at all.

Fetching in every phase also pays for the same content repeatedly: five phases reading one
card is five fetches and five copies in five contexts, for material that has not changed.

And the record matters. A specification written from a card is a decision made against what
the card said that day. When someone edits the card next month, the specification should not
change with it — it should be visibly out of step, so the difference is noticed rather than
absorbed.

## Where it goes

```
docs/changes/{change}/inputs/
  jira-PROJ-1487.md
  requirements-doc.md
  api-reference.md
```

Committed with the change, like every other artifact.

## Who does it

The phase that first receives the material: `refine` for a feature, `diagnose` for a defect.
It is their first step, before any question is asked or any investigation starts.

The orchestrator does not do it. Reading a card would put its contents in the context that
lives for the whole run, which is the one place they must not be — see
`skills/_shared/orchestrator-protocol.md`.

## Materialise, do not summarise

Write the full content as text. Strip navigation, styling and markup; keep everything that
carries meaning, including comments, acceptance criteria, attachments described in words,
and linked issue titles.

A summary is an irreversible decision made before anyone knows what will matter. The phase
that needs the detail three steps later cannot recover it, and the rule says it cannot go
back to the source. Summarising is what `refine` and `spec` do afterwards, from the full text.

## Shape

```markdown
---
source: https://jira.example.com/browse/PROJ-1487
type: tracker_card
fetched_at: 2026-09-15T10:04:00Z
fetched_by: refine
---

# PROJ-1487 — Payment retries duplicate charges

[full text, as written in the source]
```

`source` and `fetched_at` are what make the copy auditable: where it came from, and what it
said when the work was done.

## Links inside the material

A card that references another card, or a document that links to a page, is a decision, not
an automatic fetch.

```
does the referenced material change what gets built?
  yes  -> materialise it too, as its own file
  no   -> record the reference as a line of text and move on
```

Following every link reachable from the first one is how a single card turns into a
repository of someone else's wiki.

## Indexing

Each materialised input is indexed like any other artifact, under
`alfred/{change}/input/{slug}`, so subagents reach it by key rather than by path.

## What the later phases see

`refine` and `diagnose` cite the inputs they used. `spec`, `design` and every phase after
them read the materialised text and nothing else.

A phase that finds the material insufficient says so and stops. It does not fetch the
source to fill the gap: the gap is reported, and bringing in more material is a decision the
user makes, through the phase that owns ingestion.
