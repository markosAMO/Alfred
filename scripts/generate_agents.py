#!/usr/bin/env python3
"""Generate agent definitions from a model profile and the installed workflows.

One orchestrator per workflow that only delegates, and one subagent per phase that only
executes. Written for whichever agents are installed, from the same profile, so a model
change is made in one place.

A workflow is a recipe: which phases exist and in which order they run for each route.
Phases are the steps, shared across recipes. Workflows are found in two places, the ones
Alfred ships under <alfred_home>/workflows/ and the user's under
<alfred_home>/custom/workflows/, and each becomes a command of its own: /alfred-sdd,
/alfred-ventas. /alfred stays as the default workflow, so nothing already in use changes.

The orchestrator takes a different shape per agent. OpenCode has primary agents, so each
workflow becomes one. Claude Code has no primary agents to pick - its agent files are
subagents - so each workflow becomes a slash command instead. A skill would not do: a
skill loads only when the model judges it relevant, and an orchestrator has to start when
asked.

Every agent declares the tools it needs and nothing else. An agent that declares none
inherits the whole catalogue - every MCP server's full schemas - which is tens of
thousands of tokens before it reads a line. It is also what makes the orchestrator's
context budget enforceable rather than merely requested: without Bash or Grep it cannot
explore the repository, and without Edit or Write it cannot do a phase's work inline.
"""
import json
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

ORCHESTRATOR_RULES = """You are the Alfred orchestrator. You plan and delegate. You never \
do the work.

## What you may read

Exactly these, and nothing else:

  .alfred/config.yaml
  .alfred/state/*.yaml
  .alfred/skill-registry.md
  the routing rules of this workflow, named below

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
Never lengthen a route without saying so. The routes are the ones this workflow defines, \
named below; there are no others.

Delegate each phase to its subagent, passing resolved paths rather than content.

Dispatch verify and review together when both are in the route: neither reads what the \
other writes, neither writes code, and they answer independent questions. Every other pair \
is sequential.

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

MANAGE_RULES = """You are the Alfred management executor, not the orchestrator.

Do the operation yourself. Do NOT delegate. Do NOT call the Task tool. Do NOT launch \
subagents.

Read your skill at {skill_path} and run the operation named in your task: status, \
registry, doctor, reindex or add-workflow. The skill names the script each operation runs; \
run it with Bash and return its output as printed, so the orchestrator records fields \
rather than a paraphrase. Return only what the skill's completion section specifies."""

ADD_WORKFLOW_RULES = """You are adding a workflow to Alfred on this machine. You interview; \
alfred-manage writes and registers. You never write files yourself.

A workflow is a recipe: which phases it has, in which order for each route, and the one
question that decides between routes. Phases are steps; a workflow may reuse the shared
library or bring its own. Custom workflows live in `{custom_root}<name>/` and survive every
update of Alfred, because the installer never touches that directory. The definition format
is in `{protocol}`.

## If the request already contains a workflow definition

Skip the interview. Show it back, ask for confirmation once, and hand it to alfred-manage.

## Interview

One step per message. Wait for the answer before the next.

1. Name. Lowercase letters, digits and dashes; it becomes `/alfred-<name>`. Not one of
   these, which are taken: {reserved}.
2. Title, and a one-line description that will appear as the command's help.
3. Phases. The shared library offers: {library}. Ask which of those the workflow reuses,
   and which phases are new. For each new phase: what it does in one or two sentences,
   and its mode - `auto` runs without asking, `interactive` asks and waits, `confirm` asks
   permission before starting.
4. Routes. At least one: a name and the ordered phases. Which one is proposed by default.
5. The routing question: the single question whose answer chooses the route, and which
   answer leads where.
6. Show the resulting `workflow.yaml` and the list of skill files that will be created.
   Wait for a yes.

## Registering

Delegate to alfred-manage: operation `add-workflow`, with the confirmed `workflow.yaml`, the
routing question and answers, and the description and mode of every new phase. It writes
the directory from the templates, registers the commands with `{register_tool}`, and returns
what the tool printed. Relay that, and say that Claude Code must be restarted before
`/alfred-<name>` appears.

## The request

$ARGUMENTS"""

# Tools per library phase. A phase that does not write code does not get Edit; a phase
# that does not run anything does not get Bash. A workflow's own phase declares `tools:`
# in its SKILL.md frontmatter, or gets the default below.
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
    "alfred":   ["Read", "Write", "Glob", "Grep", "Bash"],
}
DEFAULT_TOOLS = ["Read", "Write", "Glob", "Grep"]

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
    "alfred":   ["mem_search", "mem_get_observation", "mem_save"],
}
DEFAULT_MEMORY = ["mem_search", "mem_get_observation", "mem_save"]

ORCHESTRATOR_TOOLS = ["Task", "Read"]

OPENCODE_NAMES = {
    "Read": "read", "Write": "write", "Edit": "edit", "Bash": "bash",
    "Glob": "glob", "Grep": "grep", "Task": "task",
    "WebFetch": "webfetch", "WebSearch": "websearch",
}

# Names a workflow cannot take: they are already commands or agents.
RESERVED_NAMES = {"manage", "worktree", "add-workflow", "alfred"}
NAME_PATTERN = re.compile(r"^[a-z][a-z0-9-]*$")


@dataclass
class Workflow:
    name: str
    title: str
    description: str
    phases: list[str]
    routes: dict[str, list[str]]
    default_route: str
    routing: Path
    entry_points: dict[str, str]
    directory: Path
    source: str
    own_phases: dict[str, Path] = field(default_factory=dict)
    own_modes: dict[str, str] = field(default_factory=dict)
    own_tools: dict[str, list[str]] = field(default_factory=dict)


class WorkflowError(Exception):
    pass


# --- parsing -------------------------------------------------------------------------

def parse_flow_list(value: str) -> list[str]:
    inner = value.strip()[1:-1]
    return [item.strip().strip("'\"") for item in inner.split(",") if item.strip()]


def parse_scalar(value: str):
    value = value.strip()
    if value.startswith("[") and value.endswith("]"):
        return parse_flow_list(value)
    return value.strip("'\"")


def parse_yaml_subset(text: str) -> dict:
    """The subset a workflow definition uses: scalars, flow lists, one level of nesting.

    A dependency on a YAML library would be the only one in the package, for a format
    whose every file is eleven lines. The subset is documented in the protocol.
    """
    result: dict = {}
    current: str | None = None
    for raw in text.splitlines():
        line = raw.rstrip()
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        indented = line.startswith(" ") or line.startswith("\t")
        key, sep, value = line.strip().partition(":")
        if not sep:
            raise WorkflowError(f"cannot parse line: {raw!r}")
        if indented:
            if current is None or not isinstance(result.get(current), dict):
                raise WorkflowError(f"indented line outside a mapping: {raw!r}")
            result[current][key.strip()] = parse_scalar(value)
        elif value.strip() == "":
            current = key.strip()
            result[current] = {}
        else:
            current = None
            result[key.strip()] = parse_scalar(value)
    return result


def parse_frontmatter(text: str) -> dict:
    if not text.startswith("---"):
        return {}
    end = text.find("\n---", 3)
    if end == -1:
        return {}
    return parse_yaml_subset(text[3:end])


# --- discovery -----------------------------------------------------------------------

def load_workflow(directory: Path, source: str, library: set[str]) -> Workflow:
    definition = directory / "workflow.yaml"
    data = parse_yaml_subset(definition.read_text())

    name = str(data.get("name", "")).strip()
    if name != directory.name:
        raise WorkflowError(f"{definition}: name {name!r} does not match its directory {directory.name!r}")
    if not NAME_PATTERN.match(name):
        raise WorkflowError(f"{definition}: name {name!r} must be lowercase letters, digits and dashes")
    if name in RESERVED_NAMES or name in library:
        raise WorkflowError(f"{definition}: name {name!r} is reserved")

    phases = data.get("phases")
    routes = data.get("routes")
    if not isinstance(phases, list) or not phases:
        raise WorkflowError(f"{definition}: phases must be a non-empty list")
    if not isinstance(routes, dict) or not routes:
        raise WorkflowError(f"{definition}: routes must be a mapping with at least one route")

    own_phases: dict[str, Path] = {}
    own_modes: dict[str, str] = {}
    own_tools: dict[str, list[str]] = {}
    missing = []
    for phase in phases:
        own = directory / "skills" / phase / "SKILL.md"
        if own.exists():
            own_phases[phase] = own
            front = parse_frontmatter(own.read_text())
            own_modes[phase] = str(front.get("mode", "auto"))
            tools = front.get("tools")
            if isinstance(tools, list) and tools:
                own_tools[phase] = tools
        elif phase not in library:
            missing.append(phase)
    if missing:
        raise WorkflowError(
            f"{definition}: phases not found in the workflow's skills/ nor in the shared library: "
            + ", ".join(missing)
        )

    for route, steps in routes.items():
        if not isinstance(steps, list) or not steps:
            raise WorkflowError(f"{definition}: route {route!r} must be a non-empty list")
        unknown = [step for step in steps if step not in phases]
        if unknown:
            raise WorkflowError(f"{definition}: route {route!r} uses phases not declared: {', '.join(unknown)}")

    default_route = str(data.get("default_route") or next(iter(routes)))
    if default_route not in routes:
        raise WorkflowError(f"{definition}: default_route {default_route!r} is not a route")

    routing = directory / str(data.get("routing", "rules.md"))
    entry_points = data.get("entry_points") if isinstance(data.get("entry_points"), dict) else {}

    return Workflow(
        name=name,
        title=str(data.get("title", name)),
        description=str(data.get("description", "")),
        phases=list(phases),
        routes={route: list(steps) for route, steps in routes.items()},
        default_route=default_route,
        routing=routing,
        entry_points=entry_points,
        directory=directory,
        source=source,
        own_phases=own_phases,
        own_modes=own_modes,
        own_tools=own_tools,
    )


def load_workflows(alfred_home: Path, library: set[str]) -> list[Workflow]:
    """Alfred's workflows first, then the user's. A name in both is an error, not a shadow.

    A custom workflow that silently replaced a shipped one would change /alfred-sdd on one
    machine and nowhere else, and nobody would know which definition ran.
    """
    workflows: list[Workflow] = []
    seen: dict[str, Path] = {}
    for root, source in ((alfred_home / "workflows", "alfred"), (alfred_home / "custom" / "workflows", "custom")):
        if not root.is_dir():
            continue
        for directory in sorted(root.iterdir()):
            if not (directory / "workflow.yaml").is_file():
                continue
            workflow = load_workflow(directory, source, library)
            if workflow.name in seen:
                raise WorkflowError(
                    f"workflow {workflow.name!r} is defined twice: {seen[workflow.name]} and {directory}. "
                    "Rename the custom one."
                )
            seen[workflow.name] = directory
            workflows.append(workflow)
    if not workflows:
        raise WorkflowError(f"no workflows found under {alfred_home}/workflows/")
    return workflows


# --- shared pieces ---------------------------------------------------------------------

def phase_tools(profile: dict, phase: str, base: list[str] | None = None) -> list[str]:
    """Base tools, memory tools with the configured prefix, then per-project extras."""
    tools = list(base or PHASE_TOOLS.get(phase, DEFAULT_TOOLS))

    prefix = profile.get("memory_tool_prefix", "mcp__engram__")
    if prefix:
        tools += [f"{prefix}{name}" for name in PHASE_MEMORY.get(phase, DEFAULT_MEMORY)]

    tools += profile.get("extra_tools", {}).get(phase, [])
    return tools


def library_phases(skills_root: str) -> list[str]:
    """The shared phases are the directories under skills/ with a SKILL.md.

    Not the profile's keys: the profile also carries `<workflow>-<phase>` assignments for
    workflows' own phases, and those are not library phases.
    """
    return sorted(
        p.name for p in Path(skills_root).iterdir()
        if (p / "SKILL.md").is_file() and p.name not in ("_shared", "alfred")
    )


def phase_model(profile: dict, phase: str) -> str:
    return profile["phases"].get(phase) or profile["orchestrator"]


def manage_model(profile: dict) -> str:
    """Management operations are bookkeeping: the init model is a sensible default."""
    return profile.get("manage") or profile["phases"].get("init") or profile["orchestrator"]


def own_phase_model(profile: dict, workflow: Workflow, phase: str) -> str:
    """A workflow's own phase is assigned as `<workflow>-<phase>`; otherwise the orchestrator's model."""
    return profile["phases"].get(f"{workflow.name}-{phase}") or profile["orchestrator"]


def effort_line(profile: dict, phase: str) -> str:
    effort = profile.get("effort", {}).get(phase)
    return f"effort: {effort}\n" if effort else ""


def subagent_name(workflow: Workflow, phase: str) -> str:
    return f"alfred-{workflow.name}-{phase}" if phase in workflow.own_phases else f"alfred-{phase}"


def workflow_section(workflow: Workflow, skills_root: str) -> str:
    routes = "\n".join(
        f"  {route:<10} {' -> '.join(steps)}" for route, steps in workflow.routes.items()
    )
    entries = "; ".join(f"{kind} -> {phase}" for kind, phase in workflow.entry_points.items()) or "none declared"
    own = ", ".join(sorted(workflow.own_phases)) or "none"
    executors = "\n".join(
        f"  {phase:<12} {subagent_name(workflow, phase)}" for phase in workflow.phases
    )
    return f"""## Workflow: {workflow.title} ({workflow.name})

{workflow.description}

Phases, in the order they may run: {', '.join(workflow.phases)}.

Routes, the only ones that exist here:

{routes}

Default route: {workflow.default_route}. How to choose is written in
`{workflow.routing}`; read it before proposing a route, and name the signals it defines.

Entry points: {entries}.

Phases this workflow brings itself: {own}. Their skills are under
`{workflow.directory}/skills/<phase>/SKILL.md`; every other phase is the shared library
under `{skills_root}/<phase>/SKILL.md`.

Executor per phase:

{executors}"""


# --- OpenCode ----------------------------------------------------------------------------

def opencode_config(profile: dict, skills_root: str, workflows: list[Workflow], default: Workflow) -> dict:
    def as_booleans(tools: list[str], allow_task: bool) -> dict:
        enabled = {OPENCODE_NAMES[t]: True for t in tools if t in OPENCODE_NAMES}
        disabled = {v: False for v in set(OPENCODE_NAMES.values()) - set(enabled)}
        entry = {**enabled, **disabled}
        entry["task"] = allow_task
        return entry

    def primary(workflow: Workflow, name: str, description: str) -> dict:
        return {
            "model": profile["orchestrator"],
            "mode": "primary",
            "description": description,
            "prompt": ORCHESTRATOR_RULES + "\n\n" + workflow_section(workflow, skills_root),
            "permission": {"task": {"*": "deny", "alfred-*": "allow"}},
            "tools": as_booleans(ORCHESTRATOR_TOOLS, allow_task=True),
        }

    agent = {
        "alfred": primary(default, "alfred", f"Alfred orchestrator, default workflow ({default.name}). Never writes code."),
    }
    for workflow in workflows:
        agent[f"alfred-{workflow.name}"] = primary(
            workflow, f"alfred-{workflow.name}", f"Alfred orchestrator: {workflow.title}. Never writes code."
        )

    for phase in library_phases(skills_root):
        agent[f"alfred-{phase}"] = {
            "model": phase_model(profile, phase),
            "mode": "subagent",
            "hidden": True,
            "description": f"Alfred {phase} phase executor",
            "prompt": SUBAGENT_RULES.format(phase=phase, skill_path=f"{skills_root}/{phase}/SKILL.md"),
            "tools": as_booleans(phase_tools(profile, phase), allow_task=False),
        }

    for workflow in workflows:
        for phase, skill in workflow.own_phases.items():
            agent[f"alfred-{workflow.name}-{phase}"] = {
                "model": own_phase_model(profile, workflow, phase),
                "mode": "subagent",
                "hidden": True,
                "description": f"Alfred {workflow.name} workflow, {phase} phase executor",
                "prompt": SUBAGENT_RULES.format(phase=phase, skill_path=str(skill)),
                "tools": as_booleans(phase_tools(profile, phase, workflow.own_tools.get(phase)), allow_task=False),
            }

    agent["alfred-manage"] = {
        "model": manage_model(profile),
        "mode": "subagent",
        "hidden": True,
        "description": "Alfred management: status, registry, doctor, reindex, add-workflow",
        "prompt": MANAGE_RULES.format(skill_path=f"{skills_root}/alfred/SKILL.md"),
        "tools": as_booleans(phase_tools(profile, "alfred"), allow_task=False),
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


# --- Claude Code ----------------------------------------------------------------------------

def agent_file(name: str, description: str, model: str, tools: list[str], body: str, effort: str = "") -> str:
    return (
        "---\n"
        f"name: {name}\n"
        f"description: {description}\n"
        f"model: {model.split('/', 1)[-1]}\n"
        + effort
        + f"tools: {', '.join(tools)}\n"
        "---\n\n"
        + body
        + "\n"
    )


def claude_agents(profile: dict, skills_root: str, claude_home: Path, workflows: list[Workflow], default: Workflow) -> dict:
    """Subagents under agents/, and one slash command per workflow under commands/.

    Stale files are removed: a workflow deleted from disk must not keep its command, and a
    phase renamed must not keep its old executor. Only alfred-* files are touched.
    """
    agents_dir = claude_home / "agents"
    commands_dir = claude_home / "commands"
    agents_dir.mkdir(parents=True, exist_ok=True)
    commands_dir.mkdir(parents=True, exist_ok=True)

    for stale in list(agents_dir.glob("alfred-*.md")) + list(commands_dir.glob("alfred*.md")):
        stale.unlink()

    written = {"subagents": 0, "commands": []}

    for phase in library_phases(skills_root):
        (agents_dir / f"alfred-{phase}.md").write_text(agent_file(
            f"alfred-{phase}", f"Alfred {phase} phase executor", phase_model(profile, phase),
            phase_tools(profile, phase),
            SUBAGENT_RULES.format(phase=phase, skill_path=f"{skills_root}/{phase}/SKILL.md"),
            effort_line(profile, phase),
        ))
        written["subagents"] += 1

    for workflow in workflows:
        for phase, skill in workflow.own_phases.items():
            name = f"alfred-{workflow.name}-{phase}"
            (agents_dir / f"{name}.md").write_text(agent_file(
                name, f"Alfred {workflow.name} workflow, {phase} phase executor",
                own_phase_model(profile, workflow, phase),
                phase_tools(profile, phase, workflow.own_tools.get(phase)),
                SUBAGENT_RULES.format(phase=phase, skill_path=str(skill)),
            ))
            written["subagents"] += 1

    (agents_dir / "alfred-manage.md").write_text(agent_file(
        "alfred-manage", "Alfred management - status, registry, doctor, reindex, add-workflow",
        manage_model(profile), phase_tools(profile, "alfred"),
        MANAGE_RULES.format(skill_path=f"{skills_root}/alfred/SKILL.md"),
    ))
    written["subagents"] += 1

    (commands_dir / "alfred.md").write_text(claude_command(profile, skills_root, default, workflows, alias=True))
    written["commands"].append("alfred")
    for workflow in workflows:
        (commands_dir / f"alfred-{workflow.name}.md").write_text(claude_command(profile, skills_root, workflow, workflows))
        written["commands"].append(f"alfred-{workflow.name}")

    (commands_dir / "alfred-add-workflow.md").write_text(claude_add_workflow_command(profile, skills_root))
    written["commands"].append("alfred-add-workflow")

    return written


def claude_command(profile: dict, skills_root: str, workflow: Workflow, workflows: list[Workflow], alias: bool = False) -> str:
    executors = sorted({subagent_name(workflow, phase) for phase in workflow.phases} | {"alfred-init", "alfred-explore"})
    others = ", ".join(f"/alfred-{other.name}" for other in workflows if other is not workflow) or "none"
    description = (
        f"Alfred orchestrator, default workflow: {workflow.title}"
        if alias else f"Alfred orchestrator: {workflow.title}"
    )

    return f"""---
description: {description}
argument-hint: [what you want done, or: init | continue | status]
model: {profile["orchestrator"].split("/", 1)[-1]}
tools: {", ".join(ORCHESTRATOR_TOOLS)}
---

{ORCHESTRATOR_RULES}

{workflow_section(workflow, skills_root)}

Other workflows installed: {others}. A request that belongs to one of them is answered by
saying so, not by running it here.

## Resolving skills

Read `.alfred/skill-registry.md` for the path of each shared skill. A repository may
override one under `.alfred/skills/`, and the override wins for that repository. When the
registry does not exist yet, shared skills are at `{skills_root}/<phase>/SKILL.md`. This
workflow's own skills are where the workflow section says.

Never read a skill by guessing its path, and never paste a skill into a subagent's prompt.

## Delegating

Each phase runs as a subagent with an empty context: {", ".join(executors)}.

`status`, `registry`, `doctor`, `reindex` and `add-workflow` are not phases. They are
operations of the alfred skill and run in alfred-manage, which has Bash and reports what
the scripts print.

Pass paths, never content. A subagent fetches what its task needs; anything pasted into its
prompt spends the clean context before the work begins.

## Starting a repository

`init` sets up the repository you are in. It is the only phase that runs before
`.alfred/` exists, so read its skill from `{skills_root}/init/SKILL.md` directly.

## The request

$ARGUMENTS
"""


def claude_add_workflow_command(profile: dict, skills_root: str) -> str:
    alfred_home = Path(skills_root).parent
    library = ", ".join(library_phases(skills_root))
    reserved = ", ".join(sorted(RESERVED_NAMES | set(library_phases(skills_root))))
    body = ADD_WORKFLOW_RULES.format(
        custom_root=f"{alfred_home}/custom/workflows/",
        protocol=f"{skills_root}/_shared/workflow-protocol.md",
        reserved=reserved,
        library=library,
        register_tool=f"{alfred_home}/bin/register.sh",
    )
    return f"""---
description: Alfred - define a custom workflow and register its /alfred-<name> command
argument-hint: [nothing, to be interviewed, or a pasted workflow.yaml]
model: {profile["orchestrator"].split("/", 1)[-1]}
tools: {", ".join(ORCHESTRATOR_TOOLS)}
---

{body}
"""


# --- entry point ----------------------------------------------------------------------------

def main() -> int:
    profile = json.loads(Path(sys.argv[1]).read_text())
    skills_root = sys.argv[2]
    alfred_home = Path(skills_root).parent
    library = set(library_phases(skills_root))

    try:
        workflows = load_workflows(alfred_home, library)
    except WorkflowError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1

    default_name = "sdd"
    config = alfred_home / "alfred.config.yaml"
    if config.is_file():
        match = re.search(r"^workflows:\n(?:[ \t]+.*\n)*?[ \t]+default:[ \t]*(\S+)", config.read_text(), re.M)
        if match:
            default_name = match.group(1)
    default = next((w for w in workflows if w.name == default_name), None)
    if default is None:
        print(f"error: default workflow {default_name!r} is not installed", file=sys.stderr)
        return 1

    written = []
    for target in sys.argv[3:]:
        kind, path = target.split("=", 1)

        if kind == "opencode":
            merge_opencode(Path(path), opencode_config(profile, skills_root, workflows, default))
            written.append(f"opencode: {len(workflows) + 1} orchestrators, alfred-manage, "
                           f"{len(library) + sum(len(w.own_phases) for w in workflows)} phase subagents")
        elif kind == "claude":
            result = claude_agents(profile, skills_root, Path(path), workflows, default)
            written.append(f"claude code: {result['subagents']} subagents, commands: "
                           + ", ".join(f"/{c}" for c in result["commands"]))

    written.append("workflows: " + ", ".join(f"{w.name} ({w.source})" for w in workflows) + f"; default {default.name}")
    print("\n".join(written))
    return 0


if __name__ == "__main__":
    sys.exit(main())
