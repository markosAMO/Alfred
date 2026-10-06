---
change: {change}
type: {feature|bug}
phase: spec
parent: {workspace:change or absent}
---

# {Change title}

## ADDED Requirements

### Requirement: {name}
The system SHALL {single obligation}.

#### Scenario: {success case}
- GIVEN {observable starting state}
- WHEN {action}
- THEN {observable outcome}

#### Scenario: {failure case}
- GIVEN {observable starting state}
- WHEN {action}
- THEN {observable outcome}

## MODIFIED Requirements

### Requirement: {existing name}
{new behaviour}
Previously: {old behaviour}

## REMOVED Requirements

### Requirement: {name}
{why, and what replaces it}

## Assumptions
Inherited from the proposal or the architecture, not decided here.

## Handoff

{Where the next phase starts: paths with lines, commands with what they printed, what was ruled out, what is still open. Fifteen lines at most, or `none`.}
