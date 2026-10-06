# AGENTS.md — operating the Alfred repository

This file is for agents that **modify Alfred itself**. It is not the `AGENTS.md` that
Alfred installs into a target project: that one tells an agent working on that project
that the pipeline is in use there.

## What Alfred is

An **installable package**, not a program. Alfred does not run the agent loop — Claude
Code, OpenCode, OpenClaw or any compatible agent does. Alfred provides the manual those
agents follow.

Practical consequence: almost everything here is Markdown. The only code is the installer
and the small tools under `bin/` that it installs, for the work an agent must not do by
hand: creating and removing git worktrees, and turning a workflow into the commands an
agent offers.

```
ALFRED REPOSITORY (the mould)        TARGET PROJECT (where the parts come out)
skills/, templates/, defaults/   ──▶ .alfred/ (changes/ included), AGENTS.md
```

Feature specs never live in Alfred. Alfred only knows how to create them.

## Repository map

| Path | Contents | Why it exists |
|---|---|---|
| `AGENTS.md` | This file | Entry point for agents modifying Alfred |
| `README.md` | Project presentation | For humans |
| `install.sh` | Installer | Copies what is needed into the target project |
| `bin/` | Tools installed with Alfred | `worktree.sh` creates, lists and removes the worktrees changes run in |
| `bin/register.sh` | Registration from inside an agent | A workflow becomes a command only when something registers it, and that must work on a machine with no clone |
| `cmd/alfred/`, `internal/` | The installer's Go helper | Writes the agent definitions, the memory server registration and the install bookkeeping |
| `internal/workflow/` | Definition, scoped scan, validation, phase and model resolution | A workflow is read and checked once, at registration; nothing parses one during a run |
| `internal/generated/` | The manifest of what registration wrote, and the local-exclude write | Removal follows what Alfred recorded generating, never a name prefix that would catch a file the user wrote |
| `alfred.config.yaml` | Default configuration | Copied to the target project and tuned there |
| `workflows/` | The workflows Alfred ships, one directory each | A workflow is the recipe a run follows; `sdd` is the one shipped |
| `skills/` | One directory per phase | The manual for each phase |
| `skills/_shared/` | Rules common to every phase | Avoids repeating the same text in 13 files |
| `memory/` | Backend-agnostic memory layer | Swap backends without touching any skill |
| `notify/` | Backend-agnostic notification layer | Same pattern, for talking to the user |
| `tracker/` | Backend-agnostic task tracker layer | Mirrors tasks to Jira, GitHub Issues, Linear or nothing |
| `templates/docs/` | Templates for `spec.md`, `design.md`, … | The shape of the output, separate from the reasoning |
| `templates/agent-pointers/` | `CLAUDE.md`, `.cursorrules`, `GEMINI.md`, … | Three-line files redirecting to `AGENTS.md` |
| `templates/workflow/` | The definition, rules file and example skill a new workflow starts from | `/alfred-add-workflow` writes a directory, not a file the user assembles by hand |
| `defaults/` | The author's default architecture | What makes Alfred personal rather than generic |
| `triggers/` | How the flow starts in each environment | OpenClaw, terminal, future entry points |
| `docs/installation.md` | The two installation levels | Machine-level setup versus repository setup |
| `docs/workflows.md` | What a workflow is and how to add one | The definition key by key, for a human writing one |
| `docs/models.md` | Model assignment and profiles | Why phases run on different models |
| `docs/artifacts.md` | Where Alfred's own documents live | A repository may not accept them, and Alfred still runs there |
| `docs/ephemeral.md` | What a change leaves behind | The documents are how phases talk; keeping them afterwards is a separate decision |
| `docs/` | Project documentation | For humans; agents do not read it |

## The `sdd` workflow

The pipeline below is one workflow's recipe, not Alfred's. It is `sdd`, the workflow Alfred
ships: its phases, routes, entry points, parallel group and closing phase are declared in
`workflows/sdd/workflow.json`, and the judgement about which route a request deserves is in
`workflows/sdd/rules.md`. A workflow written by hand declares a different set and is no less
a workflow for it, so nothing outside `workflows/sdd/` may assume these names. See
`skills/_shared/workflow-protocol.md`.

Setup and exploration belong to no workflow. `/alfred-init` and `/alfred-explore` are
commands of their own and exist on a machine where no workflow registered at all.

```
SETUP (once per project, no workflow)
  init        structure + project architecture
  explore     existing repositories only: derives the architecture from the code

ENTRY A — new feature                ENTRY B — bug
  refine      interview                 diagnose   root cause + memory lookup
  research    investigate                          ├─ behaviour changes? → spec
  spec        formal requirement                   └─ otherwise         → design

COMMON TRUNK
  design      architecture of this feature
  tasks       breakdown into small tasks
  apply       code (one subagent per task, parallel when files are disjoint)
  verify  ┐   does it satisfy the spec?
  review  ┘   is the code well written?      dispatched together
  archive     writes the record, the one document the change leaves
              feature → delta merged into master specs
              bug     → postmortem written to memory
              removes the working documents and the state file

NAVIGATION
  continue · ff · status
```

## Phase modes

Two independent axes: *does it start on its own?* and *does it talk while working?* The
phases named below are the shared library's, which is where `sdd` takes all of its own
from; a workflow's own phase answers the same two questions in its own `SKILL.md`.

| Mode | Starts on its own | Interacts | Phases |
|---|---|---|---|
| `auto` | yes | no | `research`, `spec`, `tasks`, `verify`, `review`, `archive` |
| `interactive` | yes | yes, and **waits** for an answer | `refine`, `design`, `diagnose` |
| `confirm` | no, asks first | no | `apply` |

Every mode is overridable in `alfred.config.yaml`.

## Hard rules

1. **No skill names a concrete tool.** Skills say `memory.recall(...)`, never `mem_search`.
   Translation lives in `memory/adapters/`. The same applies to `notify/`.
2. **Which side is authoritative is the repository's configuration, not the machine's.**
   Under `memory.documents: ephemeral`, the default, the documents are files while the
   change is open and `archive` removes them, leaving the record and the delta spec. Under
   `keep` they all stay, under version control, with the backend holding a disposable copy
   rebuilt by `reindex`. Under `pointer` they live in memory and the repository keeps their
   addresses. Pipeline state is a file in all three, always, per
   `skills/_shared/state-contract.md`.
3. **Alfred runs without memory, under `keep` and `ephemeral`.** With no backend configured
   the pipeline still works: subagents read the Markdown files directly. More expensive in
   tokens, never broken. Under `ephemeral` the working documents are then discarded at
   close with no second copy, which is a supported choice and one `init` states out loud.
   `pointer` is the stated exception and requires a backend that answers.
   The same applies to notification channels: an unavailable channel is skipped, never
   fatal.
4. **Subagents receive locators, not content.** A subagent starts with an empty context and
   fetches what it needs from the path or key it was handed. Pasting a four-thousand-token
   spec into every subagent defeats the purpose of a clean context. Resolving the locator
   is rule 20's business, never the subagent's.
5. **User-authored content is never overwritten.** The installer only touches what sits
   between `<!-- ALFRED:BEGIN -->` and `<!-- ALFRED:END -->`, or appends. `.alfred/state.json`
   stores the sha256 of every managed file: if it changed, the installer reports a diff
   instead of overwriting.
6. **Alfred is stack-agnostic.** Code conventions live in the target project's
   `docs/code_conventions.md`, never here. Alfred defines *what* deserves a test;
   the project defines *how* tests are run.
7. **Configuration files carry no comments.** A document that needs inline explanation is
   underspecified. Key and value names must stand on their own; explanations belong in
   `docs/`.
8. **Neutral English only**, in every file of this repository and in every message Alfred
   sends. No persona, no regional voice, no adopting the language the user wrote in.
   Documents, questions, reports and commit messages read the same way.
9. **Alfred runs at two levels.** A change spanning several repositories is planned once at
   workspace level and executed independently in each repository, which keeps its own copy
   of its slice. See `skills/_shared/workspace-protocol.md`.
10. **No skill names a model.** A skill describes the work; `models.profiles` in the
   configuration decides what executes it. See `docs/models.md`.
11. **Alfred is installed at two levels.** The orchestrator and the skills are installed
   once per machine; `init` sets up each repository from wherever it is run. An installer
   that must be copied into a repository cannot create that repository. See
   `docs/installation.md`.
12. **Skills and workflows resolve local over global.** A repository may override one skill
   without forking the rest, from `.alfred/skills/`, and may override a whole recipe the
   same way, from `.alfred/workflows/<name>/`. Skill resolution is recorded in the skill
   registry, including which source each skill came from; a workflow override is reported
   by registration, which also states the command to reach it — a repository's workflow
   colliding with a machine workflow of the same name is reached as `/alfred-<name>-local`,
   because which scope a command resolves from first was never measured. See
   `skills/_shared/skill-resolver.md` and `skills/_shared/workflow-protocol.md`.
13. **The orchestrator does no work inline and reads almost nothing.** Its working set is
   the request, the configuration, the pipeline state, the skill registry and the running
   workflow's rules file, which the workflow supplies and whose absolute path the command
   the run started from carries. It is the only participant that lives for the whole run,
   so everything it reads it carries to the end. See
   `skills/_shared/orchestrator-protocol.md`.
14. **A route is proposed, never applied silently.** The orchestrator states which signals
   it matched and waits. The user can always shorten a route; the orchestrator never
   lengthens one without saying so. See `skills/_shared/routing.md`.
15. **External material is fetched once and materialised as text.** A tracker card, a URL
   or a document from another system is read by the phase that receives it, written to
   `.alfred/changes/{change}/inputs/` in full, and never fetched again. No later phase and no
   subagent reaches the network for it. See `skills/_shared/external-inputs.md`.
16. **Parallel execution requires disjoint files, not just independent tasks.** Subagents
   share one checkout with no locking between them, so two writing the same file leave one
   silent winner. Both conditions are checked before dispatching together.
17. **Existing repositories are documented on demand.** `explore` derives specifications
   for the area a change touches, never for the whole repository. A codebase documents
   itself as it is worked on.
18. **Alfred runs in repositories that do not accept it.** `artifacts.committed: false`
   keeps Alfred's documents out of git, through the repository's local exclude file and
   never through `.gitignore`. What changes is who holds the documents, never what Alfred
   writes: a worktree is given them by copy, and closing one refuses to discard a document
   the main checkout does not already have. See `docs/artifacts.md`.
19. **A change leaves one document, and it is written before anything is deleted.**
   `archive` writes the record from `templates/docs/record.md`, reads it back, and only then
   removes the working documents under `artifacts.retain: final_only`. The record carries
   the decisions the design would otherwise take with it. A failed read-back deletes
   nothing. See `docs/ephemeral.md`.
20. **State is deleted when the change closes, and only then.** A change that completed has
   nothing to resume, so `archive` removes `.alfred/state/{change}.yaml` and stages the
   removal with the closing commit. A change that failed, was abandoned, or was closed
   incomplete keeps it: open work is work somebody may come back to. See
   `skills/_shared/state-contract.md`.
21. **A phase is told where its artifacts are; it never works it out.** The orchestrator
   resolves every locator from `memory.documents` — a path or a memory key — and passes it
   in. A phase that re-derives the mode disagrees with the run that launched it, silently,
   because reading the wrong store returns an empty result that looks like an artifact
   nobody wrote. Adding a storage mode is an orchestrator change and touches no phase. See
   `Locators` in `memory/CONTRACT.md`. Which artifacts it is handed is the running
   workflow's `reads` and `recall`, never the skill's; every phase leaves one artifact,
   named after itself and ending in a `## Handoff` the next phase starts from.
22. **A capability is wired by the installer or it is not wired.** The agents declare every
   memory tool and the server exposes every memory tool, and `install`, `update` and
   `doctor` each write or check both halves. Neither is a command in a document for someone
   to paste: a phase cannot tell a tool it was never granted from a backend that cannot do
   the thing, so a half-configured pair is silent and looks like the backend's limitation.
   See `memory/adapters/engram.md`.
23. **Parallel changes never share a checkout.** Changes started together through
   `/alfred-worktree` each run in their own git worktree on their own branch, created and
   removed by `bin/worktree.sh`, never by an agent running git by hand. One change, one
   worktree, one branch. See `skills/_shared/worktree-protocol.md`.
24. **A workflow belongs to the machine or to a repository, and nothing crosses.** The
   machine's live in `~/.config/alfred/workflows/`, shipped and managed by hash, and in
   `~/.config/alfred/custom/workflows/`, which the installer neither reads, writes nor
   removes; a repository's live in `.alfred/workflows/`. Scope is a property of the roots
   and targets a run is handed, never a branch inside it, so a machine-level operation
   cannot write into a repository and a project-level one cannot write outside it. That is
   what makes an update incapable of disturbing a repository's workflows, and it is why
   `custom/` surviving an update needs no code. See
   `skills/_shared/workflow-protocol.md`.
25. **The structural facts of a run are read once, at registration.** The definition is
   validated and rendered into the command, so nothing parses a workflow definition while a
   change is running and the only workflow file a run opens is the rules file. A definition
   edited after registration takes effect when registration runs again, and one corrupted
   after registration changes nothing about a run in progress. A workflow whose phase
   resolves nowhere, or whose routes name a phase it does not declare, is rejected there
   rather than failing three phases into a change. See
   `skills/_shared/workflow-protocol.md`.

## Commit conventions

Applies to this repository and to every project Alfred manages.

- Conventional commits: `type(scope): description`
- **No AI attribution**: never `Co-Authored-By`, `Generated with`, or any mention of a
  model. Commits are authored by the human.
- **One commit per feature**, not per task: committed once `verify` and `review` pass.
- **Never `git add .`**: only the files belonging to the feature are staged, so unrelated
  work in progress is never swept in.

## Adding a skill

1. Create `skills/<name>/SKILL.md` with frontmatter: `name`, `mode`, `skippable`, `next`.
2. Reference shared rules from `skills/_shared/` rather than repeating them. What the phase
   reads and where it writes are not in the skill: the phase leaves an artifact named
   after itself, and each workflow declares what it reads, per
   `skills/_shared/phase-protocol.md`.
3. Its artifact's template belongs in `templates/docs/`, ending in `## Handoff`.
4. Add it to a workflow's `workflow.json`, with its `reads` and `recall`.

## Adding a workflow

A workflow Alfred **ships** is a directory under `workflows/`. A workflow a user writes is
not added here at all: `/alfred-add-workflow` writes it under
`~/.config/alfred/custom/workflows/` for that machine, or under a repository's
`.alfred/workflows/` for that repository alone.

1. Create `workflows/<name>/workflow.json` from `templates/workflow/workflow.json`, with
   the keys `docs/workflows.md` lists. It is JSON because the helper decodes it with
   `encoding/json` and unknown keys are rejected; no comments, per rule 7.
2. Write `workflows/<name>/rules.md`. Every signal a route matches, every branch a phase
   takes and how a route changes mid-run belongs there, in prose, and in no second place —
   a structural fact the definition already carries must not be restated in it.
3. Declare no phase of its own. A shipped own phase would have to ship a model assignment,
   and Alfred ships none; a shipped workflow reuses the library under `skills/`.
4. Nothing is added to `PAYLOAD`: `install.sh` already installs `workflows` as a directory,
   so a new one is copied and recorded by hash with no installer change.
5. Run `./install.sh workflows` to rescan the roots and regenerate the commands, and read
   the report: a workflow that was rejected says why, and a phase with no model is named.

<!-- ALFRED:BEGIN — managed by alfred, do not edit by hand -->
## Workflow

This repository uses Alfred. Software work follows the pipeline; it does not start with
code.

### Before anything

Read `.alfred/config.yaml`, then `docs/architecture.md` and `docs/code_conventions.md`.
Resolve skills through `.alfred/skill-registry.md`; never read a skill by guessing its path.

### Routing

Choose a route and state the signals it was based on, then wait for the user to accept it.

```
behaviour unchanged        direct     apply · verify · review
behaviour changes          pipeline   spec · design · tasks · apply · verify · review · archive
request underspecified     full       refine · research · then the pipeline
defect reported            diagnose   then spec or design
```

Never lengthen a route without saying so. The user may always shorten it.

### Delegation

Each phase runs as a subagent with an empty context, receiving paths rather than content.
The orchestrator holds only the request, this configuration, the pipeline state and the
registry, and does no work inline.

### State

`.alfred/state/{change}.yaml` records the phase, the route and the channel the run started
from. `continue` resumes from it. State is updated only after a phase's document exists.

### Language

Neutral English, in documents and in messages alike, regardless of the language the
request was written in. No persona and no regional voice.

### Commits

Conventional commits, no AI attribution of any kind, one commit per change once `verify` and
`review` pass, staging only the files the work reported.
<!-- ALFRED:END -->
