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

## Workspace setup

A directory holding repositories rather than code is initialised as a workspace. It plans
changes that span repositories; it does not contain code of its own.

**Every repository below it must already be initialised.** Workspace setup does not
initialise them, and does not explore them.

```
for each directory below with a .git and no .alfred/
  report it as not initialised
```

Repositories that are not initialised are listed, and the workspace is set up without
them. Each one is initialised by running `init` inside it, which is also where the user
decides what its exploration costs.

This is what keeps the workspace cheap. Five repositories of work code would otherwise
mean five explorations in a row, triggered by a command that looked like it was only
creating directories.

### What it reads instead of exploring

Each initialised repository already states what it is, in the document its own `init`
produced.

```
<repo>/docs/architecture.md     stack, layers, persistence, external services
<repo>/docs/code_conventions.md how it is built and tested
```

Workspace `design` reads those to decide which repository does what. The information was
already derived once and confirmed by the user; deriving it again from the code would cost
more and be less accurate.

### What it creates

```
AGENTS.md
.gitignore              from templates/workspace/gitignore
docs/specs/
docs/changes/
.alfred/config.yaml     with the repository list
.alfred/state/
```

The workspace is versioned, because the system-level specifications it writes are
specifications like any other: reviewed, diffed, and shared. The `.gitignore` is an
allowlist — everything is ignored except what Alfred writes — so the child repositories
stay out of it and keep their own history.

```gitignore
/*
!.gitignore
!.alfred/
!docs/
!AGENTS.md
.alfred/skill-registry.md
```

Nesting a repository inside another without this produces a parent that either swallows the
children or reports them as untracked forever.

## Before exploring a large repository

Deriving architecture costs tokens in proportion to what is read. Report the size and let
the user choose, rather than spending it and reporting afterwards.

```
ask("This repository has ~{n} source files. Deriving the architecture now will read
     manifests, configuration and a sample of the code.",
    ["derive it now", "only the area I am about to work on", "skip, I will write it"],
    "derive it now")
```

Setting up the structure is cheap and always finishes. Deriving the architecture is the
expensive half, and it is the half that can wait: anything skipped is derived on demand,
scoped to one area, the first time a change depends on it.

Skipping leaves `docs/architecture.md` as the template from `defaults/`, which `init` then
offers to fill in by interview as it would for a new project.

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
.alfred/skill-registry.md     generated, ignored by git
.gitignore                    one line, for the registry
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

Run the registry tool named in `skills.registry_tool`:

```
~/.config/alfred/bin/registry.sh write --cwd <repository>
```

It resolves every skill, writes `.alfred/skill-registry.md` recording whether each came from
the global installation or a local override, and adds the registry to the repository's
`.gitignore`. Report what it printed. The table is not written by hand: a session hook will
rewrite it later with the same tool, and two writers with different ideas of the format
produce a registry that flips on every session. See `skills/_shared/skill-resolver.md`.

## Completion

Follow `skills/_shared/phase-protocol.md`. Report what exists now and what the first command
would be.

```
initialised: docs/, .alfred/, AGENTS.md + 4 pointers
architecture: from defaults, edited
memory: engram · tracker: none · profile: mixed
```
