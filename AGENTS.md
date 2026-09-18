# AGENTS.md — operating the Alfred repository

This file is for agents that **modify Alfred itself**. It is not the `AGENTS.md` that
Alfred installs into a target project: that one tells an agent working on that project
that the pipeline is in use there.

## What Alfred is

An **installable package**, not a program. Alfred does not run the agent loop — Claude
Code, OpenCode, OpenClaw or any compatible agent does. Alfred provides the manual those
agents follow.

Practical consequence: almost everything here is Markdown. The only code is the installer
and, under `bin/` and `scripts/`, what it installs to register workflows as commands.

```
ALFRED REPOSITORY (the mould)        TARGET PROJECT (where the parts come out)
skills/, templates/, defaults/   ──▶ .alfred/, docs/changes/, AGENTS.md
```

Feature specs never live in Alfred. Alfred only knows how to create them.

## Repository map

| Path | Contents | Why it exists |
|---|---|---|
| `AGENTS.md` | This file | Entry point for agents modifying Alfred |
| `README.md` | Project presentation | For humans |
| `install.sh` | Installer | Copies what is needed into the target project |
| `alfred.config.yaml` | Default configuration | Copied to the target project and tuned there |
| `workflows/` | One directory per shipped workflow | The recipe: which phases, in which order, for which route |
| `bin/` | Tools installed with Alfred | `register.sh` turns every workflow into a command |
| `skills/` | One directory per pipeline phase | The manual for each phase, shared by every workflow |
| `skills/_shared/` | Rules common to every phase | Avoids repeating the same text in 13 files |
| `memory/` | Backend-agnostic memory layer | Swap backends without touching any skill |
| `notify/` | Backend-agnostic notification layer | Same pattern, for talking to the user |
| `tracker/` | Backend-agnostic task tracker layer | Mirrors tasks to Jira, GitHub Issues, Linear or nothing |
| `templates/docs/` | Templates for `spec.md`, `design.md`, … | The shape of the output, separate from the reasoning |
| `templates/agent-pointers/` | `CLAUDE.md`, `.cursorrules`, `GEMINI.md`, … | Three-line files redirecting to `AGENTS.md` |
| `defaults/` | The author's default architecture | What makes Alfred personal rather than generic |
| `triggers/` | How the flow starts in each environment | OpenClaw, terminal, future entry points |
| `docs/installation.md` | The two installation levels | Machine-level setup versus repository setup |
| `docs/models.md` | Model assignment and profiles | Why phases run on different models |
| `docs/` | Project documentation | For humans; agents do not read it |

## The pipeline

```
SETUP (once per project)
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
  archive     feature → delta merged into master specs
              bug     → postmortem written to memory

NAVIGATION
  continue · ff · status
```

## Phase modes

Two independent axes: *does it start on its own?* and *does it talk while working?*

| Mode | Starts on its own | Interacts | Phases |
|---|---|---|---|
| `auto` | yes | no | `research`, `spec`, `tasks`, `verify`, `review`, `archive` |
| `interactive` | yes | yes, and **waits** for an answer | `refine`, `design`, `diagnose` |
| `confirm` | no, asks first | no | `apply` |

Every mode is overridable in `alfred.config.yaml`.

## Hard rules

1. **No skill names a concrete tool.** Skills say `memory.recall(...)`, never `mem_search`.
   Translation lives in `memory/adapters/`. The same applies to `notify/`.
2. **Files are the source of truth; the database is an index.** Specs and pipeline state
   are files under version control. The memory backend holds a searchable copy that is
   disposable and rebuildable through `reindex`.
3. **Alfred runs without memory.** With no backend configured the pipeline still works:
   subagents read the Markdown files directly. More expensive in tokens, never broken.
   The same applies to notification channels: an unavailable channel is skipped, never
   fatal.
4. **Subagents receive paths, not content.** A subagent starts with an empty context and
   fetches what it needs. Pasting a four-thousand-token spec into every subagent defeats
   the purpose of a clean context.
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
12. **Skills resolve local over global.** A repository may override one skill without
   forking the rest. Resolution is recorded in the skill registry, including which source
   each skill came from. See `skills/_shared/skill-resolver.md`.
13. **The orchestrator does no work inline and reads almost nothing.** Its working set is
   the request, the configuration, the pipeline state and the skill registry. It is the
   only participant that lives for the whole run, so everything it reads it carries to the
   end. See `skills/_shared/orchestrator-protocol.md`.
14. **A route is proposed, never applied silently.** The orchestrator states which signals
   it matched and waits. The user can always shorten a route; the orchestrator never
   lengthens one without saying so. The routes and the rules belong to the workflow:
   `workflows/sdd/rules.md` for the default one.
15. **External material is fetched once and materialised as text.** A tracker card, a URL
   or a document from another system is read by the phase that receives it, written to
   `docs/changes/{change}/inputs/` in full, and never fetched again. No later phase and no
   subagent reaches the network for it. See `skills/_shared/external-inputs.md`.
16. **Parallel execution requires disjoint files, not just independent tasks.** Subagents
   share one checkout with no locking between them, so two writing the same file leave one
   silent winner. Both conditions are checked before dispatching together.
17. **Existing repositories are documented on demand.** `explore` derives specifications
   for the area a change touches, never for the whole repository. A codebase documents
   itself as it is worked on.
18. **A workflow is a recipe; phases are the steps.** Which phases run, in which order, for
   which route is defined in `workflows/<name>/workflow.yaml`, never in a skill or in the
   orchestrator's text. Every workflow becomes `/alfred-<name>` through the same generator.
   The user's workflows live under `custom/` on the machine, which the installer never
   touches. See `skills/_shared/workflow-protocol.md`.

## Commit conventions

Applies to this repository and to every project Alfred manages.

- Conventional commits: `type(scope): description`
- **No AI attribution**: never `Co-Authored-By`, `Generated with`, or any mention of a
  model. Commits are authored by the human.
- **One commit per feature**, not per task: committed once `verify` and `review` pass.
- **Never `git add .`**: only the files belonging to the feature are staged, so unrelated
  work in progress is never swept in.

## Adding a workflow

1. Create `workflows/<name>/workflow.yaml`: name, title, description, phases, routes,
   default route, entry points. The format is in `skills/_shared/workflow-protocol.md`.
2. Write `workflows/<name>/rules.md`: the question that chooses between its routes.
3. Phases the shared library does not have go under `workflows/<name>/skills/<phase>/SKILL.md`,
   with the frontmatter of any skill.
4. Run `./install.sh workflows`. The command `/alfred-<name>` exists after the agent restarts.

A user does the same on their machine with `/alfred-add-workflow`, which writes under
`~/.config/alfred/custom/workflows/` instead.

## Adding a skill

1. Create `skills/<name>/SKILL.md` with frontmatter: `name`, `mode`, `skippable`,
   `inputs`, `outputs`.
2. Reference shared rules from `skills/_shared/` rather than repeating them.
3. If the skill produces a document, its template belongs in `templates/docs/`.
4. Register it in `alfred.config.yaml` when it is a pipeline phase.
