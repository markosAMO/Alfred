#!/usr/bin/env python3
"""Generate agent definitions from a model profile.

One orchestrator that only delegates, and one subagent per phase that only executes.
Written for whichever agents are installed, from the same profile, so a model change is
made in one place.

The orchestrator takes a different shape per agent. OpenCode has primary agents, so it
becomes one. Claude Code has no primary agents to pick - its agent files are subagents -
so it becomes the `/alfred` slash command instead. A skill would not do: a skill loads
only when the model judges it relevant, and an orchestrator has to start when asked.

Every agent declares the tools it needs and nothing else. An agent that declares none
inherits the whole catalogue - every MCP server's full schemas - which is tens of
thousands of tokens before it reads a line. It is also what makes the orchestrator's
context budget enforceable rather than merely requested: without Bash or Grep it cannot
explore the repository, and without Edit or Write it cannot do a phase's work inline.
"""
import json
import sys
from pathlib import Path

ORCHESTRATOR_RULES = """You are the Alfred orchestrator. You plan and delegate. You never \
do the work.

## What you may read

Exactly these, and nothing else:

  .alfred/config.yaml
  .alfred/state/*.yaml
  .alfred/skill-registry.md

Any other read is a violation. Specifications, designs, task lists, source files, diffs \
and test output belong to the subagents that need them. If you need to know what a \
document says, delegate to the phase that owns it and use what it returns.

You are the only participant that lives for the whole run, so everything you read you \
carry to the end. A subagent reads a document, uses it, and disappears with it. After \
eight tasks an orchestrator that read every artifact holds eight specifications and eight \
diffs, hits compaction, and loses the one thing nothing else can rebuild: the plan and \
where the run is inside it.

## What you do

Choose a route, state the signals it was based on, and wait for the user to accept it. \
Never lengthen a route without saying so.

Delegate each phase to its subagent, passing resolved paths rather than content.

Relay what a subagent returns. Do not fetch its diff to check it: verify and review exist \
to judge the work.

## Before the first phase

Check that the memory backend answers. With `memory.required: false` a backend that is
configured but not reachable degrades silently, and the run proceeds without the memory it
was supposed to have - `diagnose` finds no prior postmortems, `design` finds no prior
decisions, and neither says why.

```
backend configured and reachable    proceed
backend configured, not reachable   say so, name the backend, proceed degraded
no backend configured               proceed, no warning needed
```

Say it once, at the start. Not before every phase.

## Delegating takes time

A subagent runs for minutes on a real task. Say what you are delegating and that it will
take a while, before launching it - the harness shows no progress while it runs, and
silence is indistinguishable from a hang.

```
Delegating to alfred-design. This usually takes a few minutes.
```

## External material

Do not read it yourself. A tracker card, a URL or a document in another system is fetched \
once by the phase that receives it - refine for a feature, diagnose for a defect - and \
written to docs/changes/{change}/inputs/ as text.

If the user pastes large material into the request, have the receiving phase write it to \
inputs/ before anything else, and refer to it by path from then on."""

SUBAGENT_RULES = """You are the Alfred executor for the {phase} phase, not the \
orchestrator.

Do this phase's work yourself. Do NOT delegate. Do NOT call the Task tool. Do NOT launch \
subagents.

Read your skill at {skill_path} and follow it exactly. Read the shared protocols it \
references. Return only what your skill's completion section specifies."""

# Tools per phase. A phase that does not write code does not get Edit; a phase that does
# not run anything does not get Bash.
PHASE_TOOLS = {
    "init":     ["Read", "Write", "Edit", "Glob", "Grep", "Bash"],
    "explore":  ["Read", "Write", "Glob", "Grep", "Bash"],
    "refine":   ["Read", "Write", "Glob", "Grep", "WebFetch"],
    "research": ["Read", "Write", "Glob", "Grep", "WebSearch", "WebFetch"],
    "spec":     ["Read", "Write", "Edit", "Glob", "Grep"],
    "diagnose": ["Read", "Write", "Glob", "Grep", "Bash", "WebFetch"],
    "design":   ["Read", "Write", "Glob", "Grep"],
    "tasks":    ["Read", "Write", "Glob", "Grep"],
    "apply":    ["Read", "Write", "Edit", "Glob", "Grep", "Bash"],
    "verify":   ["Read", "Write", "Glob", "Grep", "Bash"],
    "review":   ["Read", "Write", "Glob", "Grep", "Bash"],
    "archive":  ["Read", "Write", "Edit", "Glob", "Grep", "Bash"],
}

# Memory operations per phase, from memory/CONTRACT.md. Only what each phase actually
# calls: a phase that never supersedes an entry has no use for mem_update.
PHASE_MEMORY = {
    "init":     ["mem_save"],
    "explore":  ["mem_save"],
    "refine":   ["mem_search", "mem_get_observation", "mem_save", "mem_context"],
    "research": ["mem_search", "mem_get_observation", "mem_save"],
    "spec":     ["mem_search", "mem_get_observation", "mem_save"],
    "diagnose": ["mem_search", "mem_get_observation", "mem_save", "mem_context"],
    "design":   ["mem_search", "mem_get_observation", "mem_save"],
    "tasks":    ["mem_search", "mem_get_observation", "mem_save"],
    "apply":    ["mem_search", "mem_get_observation", "mem_save", "mem_update"],
    "verify":   ["mem_search", "mem_get_observation", "mem_save"],
    "review":   ["mem_search", "mem_get_observation", "mem_save"],
    "archive":  ["mem_search", "mem_get_observation", "mem_save", "mem_update"],
}

ORCHESTRATOR_TOOLS = ["Task", "Read"]

OPENCODE_NAMES = {
    "Read": "read", "Write": "write", "Edit": "edit", "Bash": "bash",
    "Glob": "glob", "Grep": "grep", "Task": "task",
    "WebFetch": "webfetch", "WebSearch": "websearch",
}


def phase_tools(profile: dict, phase: str) -> list[str]:
    """Base tools, memory tools with the configured prefix, then per-project extras."""
    tools = list(PHASE_TOOLS.get(phase, ["Read", "Write", "Glob", "Grep"]))

    prefix = profile.get("memory_tool_prefix", "mcp__engram__")
    if prefix:
        tools += [f"{prefix}{name}" for name in PHASE_MEMORY.get(phase, [])]

    tools += profile.get("extra_tools", {}).get(phase, [])
    return tools


def effort_line(profile: dict, phase: str) -> str:
    effort = profile.get("effort", {}).get(phase)
    return f"effort: {effort}\n" if effort else ""


def opencode_config(profile: dict, skills_root: str) -> dict:
    def as_booleans(tools: list[str], allow_task: bool) -> dict:
        enabled = {OPENCODE_NAMES[t]: True for t in tools if t in OPENCODE_NAMES}
        disabled = {v: False for v in set(OPENCODE_NAMES.values()) - set(enabled)}
        entry = {**enabled, **disabled}
        entry["task"] = allow_task
        return entry

    agent = {
        "alfred": {
            "model": profile["orchestrator"],
            "mode": "primary",
            "description": "Alfred orchestrator: plans, routes and delegates. Never writes code.",
            "prompt": ORCHESTRATOR_RULES,
            "permission": {"task": {"*": "deny", "alfred-*": "allow"}},
            "tools": as_booleans(ORCHESTRATOR_TOOLS, allow_task=True),
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
            "tools": as_booleans(phase_tools(profile, phase), allow_task=False),
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
            + effort_line(profile, phase)
            + f"tools: {', '.join(phase_tools(profile, phase))}\n"
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
tools: {", ".join(ORCHESTRATOR_TOOLS)}
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
