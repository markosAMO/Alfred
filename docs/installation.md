# Installation

Alfred is installed at two levels, and the two are easy to confuse. The machine level
registers the orchestrator with your agents. The repository level sets up one project.

There is no package to download and no global `alfred` command: the installer is a script
inside this repository, and every pipeline command is given to the orchestrator from inside
an agent.

## Once per machine

```bash
git clone https://github.com/markosAMO/Alfred.git
cd Alfred
./install.sh install
```

Use the HTTPS URL unless this machine already has an SSH key on the account. The repository
is public, so HTTPS needs no credentials. If the clone did not preserve the executable bit,
run `bash install.sh install` instead.

The installer detects which agents are present, asks which model runs each phase, and
writes:

```
~/.config/alfred/                 the skills, protocols, templates and adapters
~/.config/alfred/bin/registry.sh  the skill registry tool
~/.config/alfred/profile.json     the model assignment
~/.config/alfred/state.json       the hash of every installed file
~/.config/opencode/opencode.json  orchestrator + one subagent per phase + alfred-manage
~/.config/opencode/plugins/alfred-registry.ts   runs the registry tool when a project opens
~/.claude/agents/                 one subagent per phase + alfred-manage
~/.claude/commands/alfred.md      the orchestrator, as a slash command
~/.claude/settings.json           one SessionStart hook, running the registry tool
```

Only alfred agents are written into an existing OpenCode configuration, and only one hook
entry into an existing Claude Code settings file; any other agent, hook or setting is left
as it was.

The orchestrator takes a different shape per agent, because the agents differ. OpenCode has
primary agents, so it becomes one and appears in the picker. Claude Code has no primary
agent to select — the files under `~/.claude/agents/` are subagents — so it becomes the
`/alfred` command. A skill would not do: a skill loads only when the model judges it
relevant, and an orchestrator has to start when asked.

### Installer commands

Run from inside the cloned repository.

| Command | What it does |
|---|---|
| `./install.sh install` | first-time setup |
| `./install.sh update` | refresh the installed files, leaving modified ones alone |
| `./install.sh models` | change the model profile and regenerate the agents |
| `./install.sh doctor` | check the setup and name what is broken |
| `./install.sh status` | what is installed, which profile, which agents |

`update` replaces only files whose hash still matches what was installed. A file you edited
is reported with its path, never overwritten.

To pull a newer Alfred, `git pull` in the clone and run `./install.sh update`.

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
.alfred/skill-registry.md generated, and ignored by git
.gitignore                one line, for the registry
```

An existing repository keeps whatever `AGENTS.md` already said: only the block between
`<!-- ALFRED:BEGIN -->` and `<!-- ALFRED:END -->` belongs to Alfred.

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
| `registry` | rewrite the skill registry from what is on disk |
| `doctor` | check the setup and name what is broken |
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

## The skill registry

`.alfred/skill-registry.md` is the table of every skill and the path it resolves to in this
repository. It is written by `~/.config/alfred/bin/registry.sh`, never by hand, and it is
ignored by git: it describes where skills live on one machine.

It stays current on its own. Opening a repository in an agent runs `registry.sh sync`
there, which rewrites the table only when it differs from what is on disk and prints one
line when it did. Claude Code runs it from the `SessionStart` hook the installer registered;
OpenCode runs it from the plugin the installer copied. In Claude Code the line appears in
the conversation, in OpenCode in the log. The hook never commits anything.

```
~/.config/alfred/bin/registry.sh write            rewrite when stale, report the rows that changed
~/.config/alfred/bin/registry.sh write --force    rewrite regardless
~/.config/alfred/bin/registry.sh check            exit 1 and list the stale rows
~/.config/alfred/bin/registry.sh list             print the table, write nothing
```

All of them accept `--cwd <repository>`. `write` and `sync` also add the registry to the
repository's `.gitignore` when the line is missing; `--no-gitignore` prevents that.

Per repository, `skills.registry_hook` in `.alfred/config.yaml` decides what the hook does:
`sync` (default), `check` (warn only) or `off`.

### A repository set up before the tool existed

Nothing needs re-running. After `./install.sh update` on the machine:

```bash
cd my-project
~/.config/alfred/bin/registry.sh write          # or ask the orchestrator for `registry`
git rm --cached .alfred/skill-registry.md       # only if an old registry was committed
git add .gitignore
```

`write` adds the `.gitignore` line. The `git rm --cached` stops tracking the old copy
without deleting it; commit that together with the `.gitignore` change.

### Removing the hook

Delete the `SessionStart` entry whose command contains `bin/registry.sh sync` from
`~/.claude/settings.json`, and delete `~/.config/opencode/plugins/alfred-registry.ts`.
Setting `skills.registry_hook: off` in one repository disables it there without touching
either.

## Workspaces

A directory containing several repositories is initialised the same way. `init` detects the
repositories below it and sets up the workspace level, leaving each repository free to be
initialised on its own. See `skills/_shared/workspace-protocol.md`.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `alfred: command not found` | there is no global command; run `./install.sh` from the clone |
| `Permission denied (publickey)` on clone | no SSH key on this machine; clone over HTTPS |
| `permission denied: ./install.sh` | the executable bit was lost; run `bash install.sh install` |
| `python3 is required` | the installer uses python3 for JSON and hashes |
| the orchestrator does not appear in the agent picker | that picker is OpenCode's; in Claude Code use `/alfred` |
| `/alfred` is not offered in Claude Code | no agent was detected at install time, or the session predates it — restart Claude Code |
| `.alfred/skill-registry.md` shows as modified in git | it was committed before the tool existed; see the migration steps above |
| a phase ignores a skill override in `.alfred/skills/` | the registry is stale; ask the orchestrator for `registry`, or open a new session |
| `Alfred: skill registry updated` at session start | the hook rewrote the registry because a skill was added, removed or overridden; nothing to do |

`./install.sh doctor` checks all of the above and prints the remedy for each failure.
