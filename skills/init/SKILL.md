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
.alfred/changes/
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
.alfred/changes/
.alfred/config.yaml
.alfred/state/
.alfred/skill-registry.md
```

Under `artifacts.committed: false` the same files are created and the exclusions are
written beside them. What changes is what git is holding, never what Alfred writes.

`AGENTS.md` is the source of truth; the rest are three-line files redirecting to it. Only
the block between `<!-- ALFRED:BEGIN -->` and `<!-- ALFRED:END -->` belongs to Alfred. An
existing file keeps everything it already said, per rule 5 in the package `AGENTS.md`.

## Whether the documents are committed

Asked once, here, because every later phase assumes the answer and none of them can change
it.

```
ask("Can Alfred's own documents be committed to this repository?",
    ["yes, commit them", "no, keep them out of git"], "yes, commit them")
```

`yes` is `artifacts.committed: true` and needs nothing further: the documents are versioned
with the code they describe, which is where they belong and what keeps them safe.

`no` is for a repository that will not take them — a team that has not adopted Alfred, a
review process that would reject the directory. It is a legitimate way to run, and it is
recorded rather than worked around. Alfred then writes the exclusions to
`artifacts.local_exclude`, `.git/info/exclude` by default:

```
/.alfred/
/docs/specs/
/docs/architecture.md
/docs/code_conventions.md
```

Every pattern is anchored to the repository root, because that is what is meant: the
repository's own directories, not a directory of that name at any depth under it.
Unanchored, `.alfred/` hides a vendored dependency carrying one and `docs/specs/` hides a
test fixture, and a file git is ignoring is a file that cannot be staged and never appears
in `git status`.

That file is per-clone and is not itself tracked, so keeping Alfred out of a repository
costs the repository no commit. Writing the same lines to `.gitignore` would be a change to
the thing the answer was `no` about.

Say what it costs, at the moment the answer is given rather than when it is felt: the
documents stop being versioned, nobody else's clone has them, and the only copies are this
checkout and the memory backend. A repository answering `no` is the one where a memory
backend stops being an optimisation. See `docs/artifacts.md`.

## Commit before opening a worktree

Under `artifacts.committed: true`, the last thing this phase says is that its output has to
be committed before any worktree is opened.

A worktree is created from a ref. An architecture written here and not committed is not in
any worktree made afterwards, and every phase running in one discovers its absence
separately — the cost is a rediscovery per subagent, and nothing upstream can warn them,
because the orchestrator reads the configuration, the state and the registry and nothing
else.

Under `committed: false` there is nothing to commit and the tool copies the documents into
each worktree instead, per `skills/_shared/worktree-protocol.md`.

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

**Memory follows the machine.** The installer asks once per machine whether to use a memory
backend, and the answer is the profile's `memory_tool_prefix`. An empty or absent prefix
means no agent was given a memory tool, so no phase can reach a backend whatever this file
names. Write `memory.backend: none` and `memory.adapter: memory/adapters/none.md` without
asking, and say why in one line:

```
memory: none, this machine was installed without memory (./install.sh models to enable it)
```

Naming a backend the phases cannot reach does not break a run, but it makes the
orchestrator warn at the start of every one about a backend that was never going to answer.
Only when the profile carries a prefix is the memory backend asked.

With a memory backend, also ask where the change documents live:

```
memory.documents  keep       the documents are files, memory holds a searchable copy
                  ephemeral  the documents are files while the change is open, and
                             archive removes them, leaving the record and the delta spec
                  pointer    the documents are in memory, the repository holds addresses
```

`ephemeral` is the default, and the answer for most repositories. A change writes seven or
eight documents and roughly fifteen hundred lines while it is open, and the question they
settle is answered afterwards by one record and the merged specification. It works with any
backend, including none.

`keep` keeps everything and is the answer for a repository whose history of how it was built
is wanted in the repository itself. It was the default before `ephemeral`.

`pointer` is for a repository that wants the documents kept and searchable but not on disk,
and it requires a backend that answers, per `memory/CONTRACT.md`. It cannot be chosen with
`backend: none`.

Say what `ephemeral` costs when it is chosen against `backend: none`: the working documents
are discarded at close with no second copy anywhere. That is a supported choice, and it is
the one thing about the mode a user should not discover afterwards.

Each is optional and each degrades rather than failing, per their contracts. Chosen values
are written to `.alfred/config.yaml`.

## Models

If the machine has no active profile, setup is required before any run. This phase reports
that and stops rather than assigning a default, per `docs/models.md`.

## Registry

Resolve every skill and write `.alfred/skill-registry.md`, recording whether each came from
the global installation or a local override. See `skills/_shared/skill-resolver.md`.

**Then keep it out of git.** The registry's `path` column carries machine-absolute paths
into the global installation, which resolve to nothing on anyone else's machine, so it is
regenerated per clone rather than shared.

```gitignore
.alfred/skill-registry.md
```

Added to the repository's `.gitignore` — this one is a change to the repository and belongs
there, unlike `artifacts.local_exclude`, which exists precisely to avoid one. A repository
whose `.gitignore` is an allowlist needs the negation instead, since `!.alfred/` would
otherwise pull the registry back in.

Left out, the registry shows up as an untracked file after every run and `archive` reports a
working tree it cannot explain.

## Register the repository's workflows

The last thing this phase does. A repository may bring workflows of its own under
`.alfred/workflows/`, each available in this repository and nowhere else, per
`skills/_shared/workflow-protocol.md`. Registration is what turns one into a command, and it
runs here because setting the repository up is the first moment there is a repository to
register: installing and updating the machine never walk into one, and a workflow nobody
registered is a directory with no way in.

```
~/.config/alfred/bin/register.sh --project <repository>
```

It detects the agents installed on this machine, validates every workflow it finds, writes
the commands and the subagents their phases need inside the repository, and prints what it
registered.

Relay that report as printed. It names the command each workflow is reached by, and that name
is not always the workflow's: a repository workflow carrying the name of one of the machine's
is reached as `/alfred-<name>-local`, and its own phases take the same distinguishing
form, so that `/alfred-<name>` in this repository still reaches the machine's. Which
definition runs is not in question — the repository's does, here — and the report is the
only thing that says which name reaches it.

A repository that defines no workflow has nothing to register. Nothing is written inside it
and the machine's workflows stay available here as they are everywhere else. Say that, rather
than reporting a success that created nothing.

A workflow that is rejected is reported with its reason and does not undo the setup: every
other workflow registers, and registration runs again once the definition is fixed, through
the `workflows` operation of `skills/alfred/SKILL.md`.

The exclusions are written by that run and not by this phase, per rule 22 in the package
`AGENTS.md`: an exclusion a document asks somebody to add is one somebody skips. Under
`artifacts.committed: false` every path it wrote inside the repository is appended to
`artifacts.local_exclude`, beside the lines written above. Under `true` the commands are the
repository's to commit and only the generation manifest is held back, because it records this
machine's absolute paths and the model each phase runs on, which are wrong in every other
clone.

That run reads `.alfred/config.yaml` for both settings, which is why it comes after the
configuration is written rather than beside it.

## Completion

Follow `skills/_shared/phase-protocol.md`. Report what exists now and what the first command
would be.

```
initialised: docs/, .alfred/, AGENTS.md + 4 pointers
architecture: from defaults, edited
memory: engram · tracker: none · profile: mixed
workflows: none of its own; the machine's are available here
```
