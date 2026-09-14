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

## Deriving architecture

Read structure before reading implementations: directory layout, dependency manifests,
configuration, entry points, migrations.

What it records: the stack, the layers and their boundaries, where persistence lives, which
external services are reached, and the patterns the code actually follows.

Distinguish what the code does from what it intended. A boundary crossed in three places is
a boundary with three violations, not an absent boundary. Record it as the boundary plus
the exceptions, so `review` can enforce it.

## Deriving conventions

Read the tests first. They show how tests are run, how they are named, what is mocked and
what coverage is expected, which is most of `docs/code_conventions.md`.

Then naming, file layout, error handling and linter configuration, from the code itself.

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
