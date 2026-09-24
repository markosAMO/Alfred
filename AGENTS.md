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
hand: creating and removing git worktrees.

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
| `bin/` | Tools installed with Alfred | `worktree.sh` creates, lists and removes the worktrees changes run in |
| `alfred.config.yaml` | Default configuration | Copied to the target project and tuned there |
| `skills/` | One directory per pipeline phase | The manual for each phase |
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
| `docs/artifacts.md` | Where Alfred's own documents live | A repository may not accept them, and Alfred still runs there |
| `docs/ephemeral.md` | What a change leaves behind | The documents are how phases talk; keeping them afterwards is a separate decision |
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
  archive     writes the record, the one document the change leaves
              feature → delta merged into master specs
              bug     → postmortem written to memory
              removes the working documents and the state file

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
12. **Skills resolve local over global.** A repository may override one skill without
   forking the rest. Resolution is recorded in the skill registry, including which source
   each skill came from. See `skills/_shared/skill-resolver.md`.
13. **The orchestrator does no work inline and reads almost nothing.** Its working set is
   the request, the configuration, the pipeline state and the skill registry. It is the
   only participant that lives for the whole run, so everything it reads it carries to the
   end. See `skills/_shared/orchestrator-protocol.md`.
14. **A route is proposed, never applied silently.** The orchestrator states which signals
   it matched and waits. The user can always shorten a route; the orchestrator never
   lengthens one without saying so. See `skills/_shared/routing.md`.
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
   `Locators` in `memory/CONTRACT.md`.
22. **Parallel changes never share a checkout.** Changes started together through
   `/alfred-worktree` each run in their own git worktree on their own branch, created and
   removed by `bin/worktree.sh`, never by an agent running git by hand. One change, one
   worktree, one branch. See `skills/_shared/worktree-protocol.md`.

## Commit conventions

Applies to this repository and to every project Alfred manages.

- Conventional commits: `type(scope): description`
- **No AI attribution**: never `Co-Authored-By`, `Generated with`, or any mention of a
  model. Commits are authored by the human.
- **One commit per feature**, not per task: committed once `verify` and `review` pass.
- **Never `git add .`**: only the files belonging to the feature are staged, so unrelated
  work in progress is never swept in.

## Adding a skill

1. Create `skills/<name>/SKILL.md` with frontmatter: `name`, `mode`, `skippable`,
   `inputs`, `outputs`.
2. Reference shared rules from `skills/_shared/` rather than repeating them.
3. If the skill produces a document, its template belongs in `templates/docs/`.
4. Register it in `alfred.config.yaml` when it is a pipeline phase.
