---
document: code_conventions
scope: repository
written_by: [init, explore]
---

# Code conventions

How this repository writes and verifies code. Stack-specific, so Alfred ships this empty and
`init` or `explore` fills it in.

`apply` follows it. `verify` runs the commands in it. `review` enforces it.

## Projects

| Path | Stack | Test command | Framework |
|---|---|---|---|
| `.` | | | |

One row per project. For a single repository there is one row. Commands are exact and
runnable from the repository root.

## Test layers

| Path | Layer | Available | Tool |
|---|---|---|---|
| `.` | Unit | | |
| `.` | Integration | | |
| `.` | E2E | | |

## Coverage

| Path | Command | Threshold |
|---|---|---|
| `.` | | 100% of the change |

Coverage is measured over the lines a change introduced or modified, never over the
repository. See `skills/_shared/testing-protocol.md`.

## Quality tools

| Path | Tool | Command | Enforced |
|---|---|---|---|
| `.` | Linter | | |
| `.` | Type checker | | |
| `.` | Formatter | | |

`Enforced` decides whether `verify` fails on it or `review` merely reports it.

## Naming and layout

Where files go, how they are named, how modules are bounded.

## Error handling

How errors are raised, wrapped, logged and surfaced. What is retried and what is not.

## Definition of done

What has to be true before a change is considered complete in this repository, beyond tests
passing.
