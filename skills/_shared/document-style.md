# Document style

How `spec`, `design` and `archive` write, so they can read each other.

## Requirement strength

| Keyword | Meaning |
|---|---|
| `MUST` / `SHALL` | absolute requirement; `verify` fails without it |
| `SHOULD` | recommended; a documented exception is acceptable |
| `MAY` | optional |

Prose without one of these keywords is context, not a requirement, and nothing is verified
against it.

## Requirements and scenarios

A requirement states what the system does. A scenario states how to prove it. Every
requirement carries at least one happy path and one failure path.

```markdown
### Requirement: Google sign-in

The system SHALL authenticate users through Google OAuth.

#### Scenario: successful sign-in
- GIVEN a user with a valid Google account
- WHEN they complete the OAuth consent screen
- THEN a session is created
- AND the user lands on the dashboard

#### Scenario: consent denied
- GIVEN a user who declines the consent screen
- WHEN they return to the callback
- THEN no session is created
- AND they see an explanation of what failed
```

Each scenario becomes one test, named after the scenario.

## Delta specifications

A change never rewrites a whole specification. It states what is different, so review sees
the change rather than a wall of unchanged text.

```markdown
## ADDED Requirements

### Requirement: Google sign-in
The system SHALL authenticate users through Google OAuth.

## MODIFIED Requirements

### Requirement: Session expiry
Sessions SHALL expire after 30 days of inactivity.
Previously: sessions expired after 7 days.

## REMOVED Requirements

### Requirement: Password reset by security question
Replaced by email-based reset.
```

`archive` merges these into `paths.master_specs`, which then describes current behaviour.
The pair is the point: deltas are how a change is reviewed, master specs are how the
system is understood. Keeping only one of them produces either an unreadable diff or a
document that silently goes stale.

## Frontmatter

Every generated document carries its lineage.

```yaml
---
change: login-google
type: feature
phase: spec
parent: workspace:payment-flow
created_at: 2026-09-13T05:10:00Z
---
```

`parent` is absent for single-repository work. When present, it links a repository's slice
of a change back to the system-level requirement it came from.

## Design documents

`design` answers how, for this change only. Decisions that outlive the change belong in
`paths.architecture`, not here.

```markdown
## Approach
## Components touched
## Interfaces
## Trade-offs considered
## Out of scope
```

`Trade-offs considered` records what was rejected and why. Without it, the next change
reopens a settled decision because nothing recorded that it was settled.
