# Installation

Alfred is installed at two levels, and the two are easy to confuse. The machine level
registers the orchestrator with your agents. The repository level sets up one project.

There is no package to download and no `alfred` command on your `PATH`: the installer is a
script inside this repository, and every pipeline command is given to the orchestrator from
inside an agent. The installation does keep the helper it built, at
`~/.config/alfred/bin/alfred`, but it is there for Alfred's own tools to call rather than
for you to type.

## Once per machine

```bash
git clone https://github.com/markosAMO/Alfred.git
cd Alfred
./install.sh install
```

Use the HTTPS URL unless this machine already has an SSH key on the account. The repository
is public, so HTTPS needs no credentials. If the clone did not preserve the executable bit,
run `bash install.sh install` instead.

The installer detects which agents are present, asks which model runs each phase and
whether to use Engram for memory, and writes:

```
~/.config/alfred/                         the skills, protocols, templates and adapters
~/.config/alfred/workflows/sdd/           the workflow Alfred ships
~/.config/alfred/bin/worktree.sh          creates and removes the worktrees parallel changes run in
~/.config/alfred/bin/register.sh          registers the workflows, from inside an agent
~/.config/alfred/bin/alfred               the helper this run built
~/.config/alfred/profile.json             the model assignment and the default workflow
~/.config/alfred/state.json               the hash of every installed file
~/.config/alfred/generated.json           what registration generated, so a later run can remove it
~/.config/opencode/opencode.json          one primary agent per workflow, plus the subagents its phases need
~/.claude/agents/                         one subagent per phase of every registered workflow
~/.claude/commands/alfred.md              the default workflow, as a slash command
~/.claude/commands/alfred-sdd.md          the same workflow under its own name
~/.claude/commands/alfred-worktree.md     several changes at once, as a slash command
~/.claude/commands/alfred-init.md         sets up a repository
~/.claude/commands/alfred-explore.md      derives an area's specification from existing code
~/.claude/commands/alfred-add-workflow.md the interview that creates a workflow
~/.claude/commands/alfred-workflows-scanner.md  rescans the roots and brings the commands up to date
```

Only alfred agents are written into an existing OpenCode configuration; any other agent or
setting is left as it was.

`~/.config/alfred/bin/alfred` is the one thing installed that is not copied from the
repository: it is the binary built during that run, kept because registration has to work
later on a machine where the clone was deleted. Nothing is downloaded.

**One command per workflow.** Each registered workflow becomes a command carrying its own
routes, its own entry points and its own closing phase: `/alfred-sdd` here, and
`/alfred-billing` for a workflow called `billing`. `/alfred` is whichever one
`default_workflow` in `profile.json` names, which is `sdd` unless you changed it. `/alfred-worktree` is single
and runs several changes at once on the default workflow.

Four commands belong to no workflow and exist even on a machine where nothing registered:
`/alfred-init`, `/alfred-explore`, `/alfred-add-workflow` and `/alfred-workflows-scanner`.
Setting a repository up must not depend on a recipe being installed.

They take a different shape per agent, because the agents differ. OpenCode has primary
agents, so each one appears in the picker by name. Claude Code has no primary agent to
select — the files under `~/.claude/agents/` are subagents — so each becomes a slash
command. A skill would not do: a skill loads only when the model judges it relevant, and an
orchestrator has to start when asked.

### Installer commands

Run from inside the cloned repository.

| Command | What it does |
|---|---|
| `./install.sh install` | first-time setup |
| `./install.sh update` | refresh the installed files, leaving modified ones alone |
| `./install.sh models` | change the model profile and regenerate the agents |
| `./install.sh workflows` | rescan the machine's workflow roots and regenerate the commands |
| `./install.sh doctor` | check the setup and name what is broken |
| `./install.sh status` | what is installed, which profile, which agents |

`update` replaces only files whose hash still matches what was installed. A file you edited
is reported with its path, never overwritten. It also rebuilds and re-copies the helper
from the clone it ran from.

Workflows you wrote are never touched by any of these. They live under
`~/.config/alfred/custom/workflows/`, which the installer does not list among the files it
installs, so it neither reads, writes nor removes anything there. One of them being broken
is reported during an update and does not fail it.

To pull a newer Alfred, `git pull` in the clone and run `./install.sh update`.

## The two registration levels

Registration is what turns a workflow into a command. It has the same two levels as the
installation, and they never reach into each other.

| Level | Scans | Writes into | Run by |
|---|---|---|---|
| machine | `~/.config/alfred/workflows/` and `~/.config/alfred/custom/workflows/` | `~/.claude/` and `~/.config/opencode/` | `install`, `update`, `models`, `./install.sh workflows`, `/alfred-workflows-scanner` |
| project | `<repo>/.alfred/workflows/` | that repository's `.claude/` and `opencode.json` | `init`, and `/alfred-workflows-scanner` run from the repository |

`install` and `update` never register a repository. A machine-level operation that walked
into repositories would have to find them first, and an update must not be able to disturb
a repository's own workflows.

From inside an agent, on a machine where the clone is gone:

```
~/.config/alfred/bin/register.sh                    machine scope: scan, validate, regenerate
~/.config/alfred/bin/register.sh --dry-run          the same report, writing nothing
~/.config/alfred/bin/register.sh --check            the workflows with no command
~/.config/alfred/bin/register.sh --project <repo>   the same three, for one repository
```

It shells to the installed helper and needs no Go toolchain and no source tree. See
`docs/workflows.md`.

## Once per repository

```bash
mkdir my-project && cd my-project
```

Start your agent there and ask the orchestrator to run `init`. Nothing is copied in by hand.

| Agent | How to reach the orchestrator |
|---|---|
| Claude Code | `/alfred init` |
| OpenCode | press Tab, select `alfred`, then ask for `init` |

`init` creates:

```
AGENTS.md                 with the Alfred block between markers
CLAUDE.md                 pointer to AGENTS.md
docs/architecture.md
docs/code_conventions.md
docs/specs/
docs/changes/
.alfred/config.yaml
.alfred/state/
.alfred/skill-registry.md
```

An existing repository keeps whatever `AGENTS.md` already said: only the block between
`<!-- ALFRED:BEGIN -->` and `<!-- ALFRED:END -->` belongs to Alfred.

`init` finishes by registering the repository's own workflows, if it has any, and says what
it registered. A repository that defines none has nothing written into it and keeps every
workflow the machine provides. Under `artifacts.committed: false` the generated files are
added to the repository's local exclude file, per `docs/artifacts.md`.

### Pipeline commands

These are given to the orchestrator inside an agent, not typed into a shell.

In Claude Code they follow the command: `/alfred add google sign-in`. In OpenCode you
describe the work to the selected orchestrator.

| Ask for | What happens |
|---|---|
| `init` | set up this repository |
| a description of the work | routing decides which phases run |
| `continue` | resume where the run stopped |
| `status` | which changes are open and where they are |
| `reindex` | rebuild the memory index from the files |

## Why the split

The orchestrator has to exist before the repository does. A new project starts as an empty
directory, and an installer that has to be copied into it first cannot be the thing that
sets it up.

Skills live globally so one update reaches every repository. Configuration, state and
documents live in the repository so they travel with the code, through clones, branches and
pull requests.

## Overriding a skill in one repository

```
.alfred/skills/apply/SKILL.md
```

A skill placed there wins over the global one for that repository only, and is recorded as
`local` in the skill registry. See `skills/_shared/skill-resolver.md`.

## Overriding a workflow in one repository

```
.alfred/workflows/sdd/workflow.json
.alfred/workflows/sdd/rules.md
```

A whole recipe is overridden the same way, and the repository's version is the one that
runs there. Because an agent's precedence between a project-scope command and a
machine-scope one of the same name was never measured, the repository's copy is reached
under a distinguishing name, `/alfred-sdd-local` for the example above, and registration
prints which name to use. A repository workflow whose name exists on no machine root keeps
the plain `/alfred-<name>`. See `docs/workflows.md`.

## Workspaces

A directory containing several repositories is initialised the same way. `init` detects the
repositories below it and sets up the workspace level, leaving each repository free to be
initialised on its own. See `skills/_shared/workspace-protocol.md`.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `alfred: command not found` | nothing is put on your `PATH`; run `./install.sh` from the clone, and let Alfred's own tools call `~/.config/alfred/bin/alfred` |
| a workflow you wrote has no command | it was never registered, or it was rejected; run `./install.sh workflows` and read the report |
| `/alfred` says the default workflow is not installed | `default_workflow` in `profile.json` names a workflow no root holds; install it or run `./install.sh models` |
| `Permission denied (publickey)` on clone | no SSH key on this machine; clone over HTTPS |
| `permission denied: ./install.sh` | the executable bit was lost; run `bash install.sh install` |
| `go is required` | the installer builds its helper from `cmd/alfred`; install Go and run it again |
| the orchestrator does not appear in the agent picker | that picker is OpenCode's; in Claude Code use `/alfred` |
| `/alfred` is not offered in Claude Code | no agent was detected at install time, or the session predates it — restart Claude Code |

`./install.sh doctor` checks all of the above and prints the remedy for each failure.
