#!/usr/bin/env python3
"""Generate agent definitions from a model profile.

One orchestrator that only delegates, and one subagent per phase that only executes.
Written for whichever agents are installed, from the same profile, so a model change is
made in one place.

The orchestrator takes a different shape per agent. OpenCode has primary agents, so it
becomes one. Claude Code has no primary agents to pick - its agent files are subagents -
so it becomes the `/alfred` slash command instead. A skill would not do: a skill loads
only when the model judges it relevant, and an orchestrator has to start when asked.
"""
import json
import sys
from pathlib import Path

ORCHESTRATOR_RULES = """You are the Alfred orchestrator. You plan and delegate; you never \
do work inline.

Hold only the request, .alfred/config.yaml, .alfred/state/, and .alfred/skill-registry.md. \
Do not read specifications, designs, diffs or test output: a subagent reads those and \
disappears with them, while anything you read you carry until the run ends.

Choose a route, state the signals it was based on, and wait for the user to accept it. \
Never lengthen a route without saying so.

Delegate each phase to its subagent, passing resolved paths rather than content."""

SUBAGENT_RULES = """You are the Alfred executor for the {phase} phase, not the \
orchestrator.

Do this phase's work yourself. Do not delegate, do not call task or delegate tools, and do \
not launch subagents.

Read your skill at {skill_path} and follow it exactly. Read the shared protocols it \
references. Return only what your skill's completion section specifies."""


def opencode_config(profile: dict, skills_root: str) -> dict:
    agent = {
        "alfred": {
            "model": profile["orchestrator"],
            "mode": "primary",
            "description": "Alfred orchestrator: plans, routes and delegates. Never writes code.",
            "prompt": ORCHESTRATOR_RULES,
            "permission": {"task": {"*": "deny", "alfred-*": "allow"}},
            "tools": {"read": True, "write": True, "edit": True, "bash": True, "task": True},
        }
    }

    for phase, model in profile["phases"].items():
        skill_path = f"{skills_root}/{phase}/SKILL.md"
        agent[f"alfred-{phase}"] = {
            "model": model,
            "mode": "subagent",
            "hidden": True,
            "description": f"Alfred {phase} phase executor",
            "prompt": SUBAGENT_RULES.format(phase=phase, skill_path=skill_path),
            "tools": {"read": True, "write": True, "edit": True, "bash": True, "task": False},
        }

    return {"$schema": "https://opencode.ai/config.json", "agent": agent}


def merge_opencode(target: Path, generated: dict) -> None:
    """Replace only the alfred-* agents, leaving any other agent untouched."""
    existing = json.loads(target.read_text()) if target.exists() else {}
    agents = {
        name: definition
        for name, definition in existing.get("agent", {}).items()
        if name != "alfred" and not name.startswith("alfred-")
    }
    agents.update(generated["agent"])
    existing["agent"] = agents
    existing.setdefault("$schema", generated["$schema"])
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(existing, indent=2) + "\n")


def claude_agents(profile: dict, skills_root: str, claude_home: Path) -> int:
    """Twelve subagents under agents/, and the orchestrator as a slash command."""
    agents_dir = claude_home / "agents"
    agents_dir.mkdir(parents=True, exist_ok=True)
    written = 0

    for phase, model in profile["phases"].items():
        skill_path = f"{skills_root}/{phase}/SKILL.md"
        body = (
            "---\n"
            f"name: alfred-{phase}\n"
            f"description: Alfred {phase} phase executor\n"
            f"model: {model.split('/', 1)[-1]}\n"
            "---\n\n"
            + SUBAGENT_RULES.format(phase=phase, skill_path=skill_path)
            + "\n"
        )
        (agents_dir / f"alfred-{phase}.md").write_text(body)
        written += 1

    commands_dir = claude_home / "commands"
    commands_dir.mkdir(parents=True, exist_ok=True)
    (commands_dir / "alfred.md").write_text(
        claude_command(profile, skills_root, sorted(profile["phases"]))
    )

    return written + 1


def claude_command(profile: dict, skills_root: str, phases: list[str]) -> str:
    subagents = ", ".join(f"alfred-{phase}" for phase in phases)

    return f"""---
description: Alfred orchestrator - plan, route and delegate a change
argument-hint: [what you want done, or: init | continue | status]
model: {profile["orchestrator"].split("/", 1)[-1]}
---

{ORCHESTRATOR_RULES}

## Resolving skills

Read `.alfred/skill-registry.md` for the path of each skill. A repository may override a
skill under `.alfred/skills/`, and the override wins for that repository. When the registry
does not exist yet, skills are at `{skills_root}/<phase>/SKILL.md`.

Never read a skill by guessing its path, and never paste a skill into a subagent's prompt.

## Delegating

Each phase runs as a subagent with an empty context: {subagents}.

Pass paths, never content. A subagent fetches what its task needs; anything pasted into its
prompt spends the clean context before the work begins.

## Starting a repository

`init` sets up the repository you are in. It is the only phase that runs before
`.alfred/` exists, so read its skill from `{skills_root}/init/SKILL.md` directly.

## The request

$ARGUMENTS
"""


def main() -> int:
    profile = json.loads(Path(sys.argv[1]).read_text())
    skills_root = sys.argv[2]
    written = []

    for target in sys.argv[3:]:
        kind, path = target.split("=", 1)

        if kind == "opencode":
            merge_opencode(Path(path), opencode_config(profile, skills_root))
            written.append(f"opencode: {len(profile['phases']) + 1} agents")
        elif kind == "claude":
            count = claude_agents(profile, skills_root, Path(path))
            written.append(f"claude code: {count - 1} subagents + /alfred command")

    print("\n".join(written))
    return 0


if __name__ == "__main__":
    sys.exit(main())
