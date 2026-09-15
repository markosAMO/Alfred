---
name: explore
mode: auto
runs_when: repository_has_existing_code
scope: area_under_change
reads: [code, tests, architecture, memory]
writes: [architecture, conventions, master_specs]
next: []
---

# explore

Derive what the code already says, and write it down where the pipeline can read it.

This phase changes no code.

## Two scopes

```
repository   once, when Alfred is added to an existing codebase
             derives architecture and conventions

area         whenever spec needs a requirement that was never written down
             derives the master specification for that area only
```

The area scope is the one that runs most. A codebase documents itself as it is worked on,
and deriving specifications for two hundred untouched features costs hours and produces
documents nobody reads.

## Reading budget

Most of what this phase records lives in a handful of small files. Reading the
implementation to find it is the expensive way to learn what a manifest states outright.

Read in tiers, and stop at the tier that answers the question.

**Tier 1 — read in full. Small, dense, and usually enough.**

```
dependency manifests    Gemfile, Gemfile.lock, package.json, go.mod, pyproject.toml
configuration           config/, initializers, settings, .env.example
containers and CI       Dockerfile, compose files, pipeline definitions
schema                  migrations directory listing, schema dump
layout                  the directory tree, two or three levels, names only
existing documentation  README, docs/, architecture notes already written
```

A Rails repository's internal gems, its authorization approach, its background job runner
and its database all appear here, in files measured in kilobytes.

**Tier 2 — sample, never sweep.** One representative per layer or pattern, plus its tests.
Three controllers that agree tell you the convention; the fourth tells you nothing new.

**Tier 3 — targeted search only.** For a question the first two tiers left open, search for
the term rather than reading the files that might contain it.

## Stopping

Stop when reading stops changing the document.

Each file read either adds something to the architecture or confirms what is already
written. Once a tier produces only confirmations, the next file in it will too, and the
tier is finished.

A large repository does not need a proportionally larger exploration. This document exists
so `design` does not contradict what is already true; it is not an inventory of the
codebase. Whatever it leaves out is derived on demand, scoped to the area under change, the
first time a change depends on it.

## Deriving architecture

What it records: the stack, the layers and their boundaries, where persistence lives, which
external services are reached, and the patterns the code actually follows.

Distinguish what the code does from what it intended. A boundary crossed in three places is
a boundary with three violations, not an absent boundary. Record it as the boundary plus
the exceptions, so `review` can enforce it.

## Deriving conventions

Read the test configuration and one test file per layer. The runner, the naming, what is
mocked and what coverage is expected are most of `docs/code_conventions.md`, and they come
from configuration plus a sample, not from the suite.

Then naming, file layout, error handling and linter configuration, from the linter config
first and the code only where the config is silent.

Where the codebase is inconsistent, record the dominant pattern and note the exception
rather than picking silently. A convention derived from the minority becomes a review
finding on every future change.

## Deriving a master specification

For an area under change, reconstruct what the system currently does, in the shape of
`skills/_shared/document-style.md`: requirements with scenarios.

```
tests           the scenarios that already exist, named
code paths      the branches no test covers
error handling  the failure behaviour, which is usually undocumented
```

Behaviour with no test is still behaviour, and still gets a requirement. Mark it
`unverified`, so `spec` knows it was read from the code rather than confirmed by a test,
and `verify` does not treat it as an established guarantee.

## Confirmation

Derived documents are presented before being written. Inference from code is a reading, and
a wrong reading recorded as architecture constrains every change after it.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the documents, index them, notify
`phase_completed`.

```
explored: auth area, 4 requirements derived, 2 unverified
```
