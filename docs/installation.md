# Installation

Alfred is installed twice, at two different levels. Conflating them is the most common way
to end up reinstalling a package in every repository.

## Once per machine

```
alfred install
```

Registers Alfred with the agents present on the machine and installs the skills globally.

```
~/.config/alfred/skills/              the skills
~/.config/opencode/opencode.json      agent definitions, one per phase
~/.claude/agents/                     the same definitions for Claude Code
```

After this, the orchestrator is available in any directory. In OpenCode it appears in the
agent picker; in Claude Code it is available as a subagent.

Rerunning it updates the global skills and regenerates the agent definitions. Files
carrying local modifications are reported rather than overwritten.

## Once per repository

```
mkdir my-project && cd my-project
```

Then start the orchestrator and run `init`. Nothing is installed by hand.

`init` creates the structure in whatever repository it is run from:

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

## Why the split

The orchestrator has to exist before the repository does. A new project starts as an empty
directory, and an installer that has to be copied into it first cannot be the thing that
sets it up.

Skills live globally so one update reaches every repository. Per-repository configuration,
state and documents live in the repository so they travel with the code, through clones,
branches and pull requests.

## Overriding a skill in one repository

```
.alfred/skills/apply/SKILL.md
```

A skill placed there wins over the global one for that repository only, and is recorded as
`local` in the skill registry. See `skills/_shared/skill-resolver.md`.

## Workspaces

A directory containing several repositories is initialised the same way. `init` detects
the repositories below it and sets up the workspace level, leaving each repository free to
be initialised on its own. See `skills/_shared/workspace-protocol.md`.
