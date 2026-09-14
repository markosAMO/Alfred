---
name: init
mode: interactive
runs: once_per_project
reads: [defaults, memory]
writes: [architecture, conventions, agent_files, local_config]
next: []
---

# init

Set up Alfred in the repository it is run from. Nothing is copied in by hand: the
orchestrator already exists on the machine, per `docs/installation.md`.

## Detect first

```
does the directory contain repositories rather than code?  -> workspace setup
does the repository already contain code?                  -> run explore first
is Alfred already initialised here?                        -> update, never overwrite
```

An existing repository has architecture and conventions already, expressed in its code.
Interviewing the user about them when `explore` can derive them and ask for confirmation
wastes their time and gets worse answers.

## What it creates

```
AGENTS.md                     Alfred block between markers
CLAUDE.md                     pointer
.cursorrules                  pointer
GEMINI.md                     pointer
.github/copilot-instructions.md   pointer
docs/architecture.md
docs/code_conventions.md
docs/specs/
docs/changes/
.alfred/config.yaml
.alfred/state/
.alfred/skill-registry.md
```

`AGENTS.md` is the source of truth; the rest are three-line files redirecting to it. Only
the block between `<!-- ALFRED:BEGIN -->` and `<!-- ALFRED:END -->` belongs to Alfred. An
existing file keeps everything it already said, per rule 5 in the package `AGENTS.md`.

## Architecture

`defaults/architecture.md` is the starting point. It is shown, and the user chooses to keep
it, edit it, or replace it.

```
ask("Use the default architecture, edit it, or write your own?",
    ["keep", "edit", "replace"], "keep")
```

What it records: stack, layers, boundaries, persistence, external services, and the
decisions that constrain future work. Not the current feature set.

For an existing repository this section is filled by `explore` and presented for
confirmation rather than asked.

## Conventions

`docs/code_conventions.md` is stack-specific and therefore never shipped with content.
Alfred is stack-agnostic, per rule 6.

The questions it must answer:

```
how are tests run, exactly which command
how is coverage measured
naming, file layout, module boundaries
linter and formatter, and whether they are enforced
error handling conventions
what "done" means in this repository
```

The test command matters more than the rest: `verify` runs it, and a wrong command makes
every verification an environment failure.

## Backends

```
memory    engram | a remote database | none
notify    terminal always; openclaw optional
tracker   none | jira | github-issues | linear
```

Each is optional and each degrades rather than failing, per their contracts. Chosen values
are written to `.alfred/config.yaml`.

## Models

If the machine has no active profile, setup is required before any run. This phase reports
that and stops rather than assigning a default, per `docs/models.md`.

## Registry

Resolve every skill and write `.alfred/skill-registry.md`, recording whether each came from
the global installation or a local override. See `skills/_shared/skill-resolver.md`.

## Completion

Follow `skills/_shared/phase-protocol.md`. Report what exists now and what the first command
would be.

```
initialised: docs/, .alfred/, AGENTS.md + 4 pointers
architecture: from defaults, edited
memory: engram · tracker: none · profile: mixed
```
