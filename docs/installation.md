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
~/.config/alfred/                      the skills, protocols, templates and adapters
~/.config/alfred/bin/worktree.sh       creates and removes the worktrees parallel changes run in
~/.config/alfred/profile.json          the model assignment
~/.config/alfred/state.json            the hash of every installed file
~/.config/opencode/opencode.json       both orchestrators + one subagent per phase
~/.claude/agents/                      one subagent per phase
~/.claude/commands/alfred.md           one change, as a slash command
~/.claude/commands/alfred-worktree.md  several changes at once, as a slash command
```

Only alfred agents are written into an existing OpenCode configuration; any other agent or
setting is left as it was.

There are two orchestrators, the same pair on both agents: one change in the checkout you
are in, and several at once with a worktree each. They take a different shape per agent,
because the agents differ. OpenCode has primary agents, so they become `alfred` and
`alfred-worktree` in the picker. Claude Code has no primary agent to select — the files
under `~/.claude/agents/` are subagents — so they become the `/alfred` and
`/alfred-worktree` commands. A skill would not do: a skill loads only when the model judges
it relevant, and an orchestrator has to start when asked.

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
.alfred/skill-registry.md
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

`./install.sh doctor` checks all of the above and prints the remedy for each failure.
