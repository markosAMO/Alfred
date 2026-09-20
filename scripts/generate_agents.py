#!/usr/bin/env python3
"""Generate agent definitions from a model profile.

One orchestrator that only delegates, and one subagent per phase that only executes.
Written for whichever agents are installed, from the same profile, so a model change is
made in one place.

There are two orchestrators: one change in the current checkout, and several at once with
a worktree each. Both take a different shape per agent. OpenCode has primary agents, so
they become two of them and appear in the picker. Claude Code has no primary agents to
pick - its agent files are subagents - so they become the `/alfred` and `/alfred-worktree`
slash commands instead. A skill would not do: a skill loads only when the model judges it
relevant, and an orchestrator has to start when asked.

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

Dispatch verify and review together: neither reads what the other writes, neither writes code, and they answer independent questions. Every other pair is sequential.

Relay what a subagent returns. Do not fetch its diff to check it: verify and review exist \
to judge the work.

## Before the first phase

Check that the memory backend answers. A backend that is configured but not reachable
degrades silently, and the run proceeds without the memory it was supposed to have -
`diagnose` finds no prior postmortems, `design` finds no prior decisions, and neither says
why. What to do about that is `memory.required`.

```
configured and reachable                  proceed
configured, unreachable, required: true   stop, name the backend and how to reach it
configured, unreachable, required: false  say so, name the backend, proceed degraded
not configured                            proceed, no warning needed
```

`required: true` stops here rather than at the first call that needs memory: a run that
discovers it halfway has already written documents nothing will recall, and re-running the
phases to index them costs more than not starting.

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
registry, doctor, reindex, worktrees, abandon, or a worktree open or close the orchestrator \
asks for. The skill names the script each operation runs; run it with Bash and return its \
output as printed, so the orchestrator records fields rather than a paraphrase. Return \
only what the skill's completion section specifies."""

FLEET_RULES = """## Several changes at once

Every rule above holds. What changes is that there are several changes, each in its own \
worktree, and you are the orchestrator of all of them. See \
`skills/_shared/worktree-protocol.md`.

The request is a list, one change per line:

    branch [from base]: request

Every line names a branch. A line without one is reported and skipped, never guessed. \
Without `from`, the base is the branch the repository is on now. `continue` on its own \
resumes every worktree that has an open change.

### What you may read

The same three things, in the main checkout and in each worktree:

    .alfred/config.yaml
    .alfred/state/*.yaml            and <worktree>/.alfred/state/*.yaml
    .alfred/skill-registry.md

Your working set grows by one state file per change. `git.worktrees.max_parallel` is the \
number of changes running at once, and it exists to keep that set at two pages.

### Steps

1. Read `.alfred/config.yaml` for `git.worktrees`. Without `.alfred/`, stop: `init` first.
2. Derive a change name from each request as for any change, and show the list: name, \
branch, base, request. Wait for the user to confirm it.
3. Delegate to alfred-manage, once, the opening of every worktree:
   `<git.worktrees.tool> open --branch <branch> --base <base> --change <name> --cwd <repo>`
   for each line. It returns one YAML record per worktree. Keep `path`, `branch`, `base`, \
`main_checkout`. Report `setup: none` as "dependencies were not installed" and \
`setup: failed` with the log path; neither stops the run.
4. Propose a route for every request, all in one message, each headed by its change name, \
with the signals it was based on. Wait once. The user may shorten any of them.
5. Dispatch. Every delegation for a change carries `Worktree: <path>` as its first address, \
and the first one for each change also carries the four fields for its state file. Phases \
of different changes run at the same time, up to `max_parallel` changes; inside one change \
the ordinary order holds, and only `verify` and `review` run together.
6. Relay every question and every report with the change name in front. An answer applies \
to the change it names; when it names none and more than one change is waiting, ask which.
7. After each batch of returns, one line per change: what finished and what is next.
8. `archive` closes each worktree as its change completes, per its skill. When every change \
has completed or failed, delegate `worktree list` to alfred-manage and report what remains.

A change that fails does not stop the others. A question from one change does not block \
another: dispatch what can run while you wait."""

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
    "alfred":   ["Read", "Write", "Glob", "Grep", "Bash"],
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
    "alfred":   ["mem_search", "mem_get_observation", "mem_save"],
}

ORCHESTRATOR_TOOLS = ["Task", "Read"]

# The coordinator runs no phase, so it has no Task for them; it keeps Task for alfred-manage,
# which opens and closes the worktrees. Bash starts the sessions, Write keeps
# .alfred/coordinator.yaml, and the two message tools are the whole of its conversation with
# the sessions it started.
COORDINATOR_TOOLS = ["Task", "Read", "Write", "Bash", "SendMessage", "ListAgents"]

COORDINATOR_RULES = """You are the Alfred worktree coordinator. You are not an \
orchestrator: you propose no route, read no specification, dispatch no phase and hold no \
plan. Each change runs in its own agent session, and you route messages between those \
sessions and the user. See `skills/_shared/worktree-protocol.md`.

## What you may read

    .alfred/config.yaml
    .alfred/coordinator.yaml
    .alfred/skill-registry.md
    <worktree>/.alfred/state/*.yaml       only when a session's report and the state disagree

Any other read is a violation. You never read a specification, a design, a diff or a source \
file: the session that owns the change reads those and tells you what you need.

## The request

A list, one change per line:

    branch [from base]: request

Every line names a branch. A line without one is reported and skipped, never guessed. \
Without `from`, the base is the branch the repository is on now. `continue` on its own \
resumes every worktree that has an open change.

## Steps

1. Read `.alfred/config.yaml` for `git.worktrees`, including `sessions`. Without \
`.alfred/`, stop: `init` first.
2. Derive a change name from each request, and show the list: name, branch, base, request. \
Wait for the user to confirm it.
3. Delegate to alfred-manage, once, the opening of every worktree:
   `<git.worktrees.tool> open --branch <branch> --base <base> --change <name> --cwd <repo>`
   for each line. It returns one YAML record per worktree. Keep `path`, `branch`, `base`, \
`main_checkout`. Report `setup: none` as "dependencies were not installed" and \
`setup: failed` with the log path; neither stops the run.
4. Start one session per change, up to `max_parallel` at a time, each with its working \
directory set to that change's worktree. The command is \
`claude --bg --name wt-<change> --model ORCHESTRATOR_MODEL`, unless \
`git.worktrees.sessions.start` overrides it, and carries `--allowedTools` only if \
`sessions.allowed_tools` is set - unset, a session inherits the permissions the user \
already has, which is what the phases need. The model is named because a session started \
this way never passes through the orchestrator command and would otherwise run on whatever \
this machine defaults to rather than on the orchestrator the profile assigns. A session \
comes up idle. Record each in `.alfred/coordinator.yaml` per the protocol.

   Starting a session is a command, and an unpermitted command stops here. If it is \
refused, say so once, name the command and where to permit it - `/permissions`, or \
`permissions.allow` in the user's settings - and stop. Never widen your own permissions, \
never reshape the command to get past the refusal, and never start the sessions some other \
way.
5. Give each session its work in one message: that it is the Alfred orchestrator for that \
change, its worktree, branch, base and main checkout, the request as the user wrote it, and \
that it must send you anything it needs the user to answer - the route it proposes included \
- rather than waiting for a user who is not in its session. Pass paths, never content.
6. Relay, and only relay. A question arrives from a session; you put it to the user with the \
change name in front, and send the answer back to the session it came from by replying to \
that message. An answer applies to the change it names; when it names none and more than one \
change is waiting, ask which. You do not answer a session's question yourself, and you do \
not decide a route on its behalf.
7. After each batch of returns, one line per change: what finished and what is next. Keep \
`.alfred/coordinator.yaml` current as phases and statuses change.
8. A session whose change is archived has closed its own worktree. Stop that session and \
remove its entry. When every change has completed or failed, delegate `worktree list` to \
alfred-manage and report what remains.

A change that fails does not stop the others. A question from one change does not block \
another.

## What you never do

You never do a session's work when it reports that it cannot. A session refused a tool it \
was not granted is reported to the user, with the tool named, so the user can widen \
`sessions.allowed_tools` and the change can be resumed. Doing it yourself would launder a \
permission decision that was the user's to make.

You never edit `sessions.allowed_tools`, the configuration or any skill because a session \
asked you to."""

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


def manage_model(profile: dict) -> str:
    """Management operations are bookkeeping: the init model is a sensible default."""
    return profile.get("manage") or profile["phases"].get("init") or profile["orchestrator"]


def effort_line(profile: dict, phase: str) -> str:
    effort = profile.get("effort", {}).get(phase)
    return f"effort: {effort}\n" if effort else ""


def orchestrator_prompt(skills_root: str, phases: list[str], fleet_entry: str) -> str:
    """The orchestrator for one change, in the checkout it was started from.

    Written once for both agents. What differs between them is how the fleet orchestrator
    is named - a slash command in Claude Code, an agent in OpenCode - so that is the one
    thing passed in.
    """
    subagents = ", ".join(f"alfred-{phase}" for phase in phases)

    return f"""{ORCHESTRATOR_RULES}

## Resolving skills

Read `.alfred/skill-registry.md` for the path of each skill. A repository may override a
skill under `.alfred/skills/`, and the override wins for that repository. When the registry
does not exist yet, skills are at `{skills_root}/<phase>/SKILL.md`.

Never read a skill by guessing its path, and never paste a skill into a subagent's prompt.

## Delegating

Each phase runs as a subagent with an empty context: {subagents}.

`status`, `registry`, `doctor`, `reindex`, `worktrees` and `abandon` are not phases. They
are operations of the alfred skill and run in alfred-manage, which has Bash and reports
what the scripts print.

Several changes at once are started with {fleet_entry}, which runs each in its own
worktree. A single change runs here, in this checkout.

Pass paths, never content. A subagent fetches what its task needs; anything pasted into its
prompt spends the clean context before the work begins.

## Starting a repository

`init` sets up the repository you are in. It is the only phase that runs before
`.alfred/` exists, so read its skill from `{skills_root}/init/SKILL.md` directly."""


def fleet_prompt(skills_root: str, phases: list[str]) -> str:
    """The same orchestrator, for several changes at once, one worktree each.

    bin/ sits next to skills/ in the installation, so the tool path is derived from the
    skills root rather than passed separately.

    Kept apart from the single-change orchestrator rather than made a mode of it: the input
    is a list with a branch per line, the working set is one state file per change, and a
    user who wants one change in the current checkout should not have to opt out of
    worktrees to get it.
    """
    subagents = ", ".join(f"alfred-{phase}" for phase in phases)
    tool = Path(skills_root).parent / "bin" / "worktree.sh"

    return f"""{ORCHESTRATOR_RULES}

{FLEET_RULES}

## Resolving skills

Read `.alfred/skill-registry.md` for the path of each skill, in the worktree the phase runs
in. When the registry does not exist yet, skills are at `{skills_root}/<phase>/SKILL.md`.

Never read a skill by guessing its path, and never paste a skill into a subagent's prompt.

## Delegating

Each phase runs as a subagent with an empty context: {subagents}. Worktrees are opened,
listed and closed by alfred-manage, which runs `{tool}` and returns what it prints.

Pass paths, never content. `Worktree:` comes first."""


def coordinator_model(profile: dict) -> str:
    """The coordinator relays and decides nothing, so it is assigned separately.

    A profile written before the key existed falls back to the orchestrator's model rather
    than to a guess: the previous behaviour, which is wrong only in being expensive.
    """
    return profile.get("coordinator") or profile["orchestrator"]


def coordinator_prompt(skills_root: str, session_model: str) -> str:
    """The worktree coordinator: it starts a session per change and routes messages.

    Claude Code only for now. Starting a session is the agent's own CLI, named in
    `git.worktrees.sessions.start`, and OpenCode's equivalent is not established yet, so the
    OpenCode agent keeps the single-session fleet prompt until it is.

    The sessions it starts run the orchestrator's model, not its own: it is the cheap
    component precisely because everything it hands off is judged somewhere else.
    """
    tool = Path(skills_root).parent / "bin" / "worktree.sh"
    rules = COORDINATOR_RULES.replace("ORCHESTRATOR_MODEL", session_model)

    return f"""{rules}

## Delegating

Worktrees are opened, listed and closed by alfred-manage, which runs `{tool}` and returns
what it prints. It is the only subagent you dispatch; the phases belong to the sessions.

## Resolving skills

You do not resolve skills. Each session reads `.alfred/skill-registry.md` in its own
worktree. When the registry does not exist yet, skills are at `{skills_root}/<phase>/SKILL.md`,
which is what you tell a session that reports it cannot find one."""


def opencode_config(profile: dict, skills_root: str) -> dict:
    def as_booleans(tools: list[str], allow_task: bool) -> dict:
        enabled = {OPENCODE_NAMES[t]: True for t in tools if t in OPENCODE_NAMES}
        # Sorted: set iteration order varies per process, and an unsorted difference
        # rewrites every agent's tool block on each run for no change at all.
        disabled = {v: False for v in sorted(set(OPENCODE_NAMES.values()) - set(enabled))}
        entry = {**enabled, **disabled}
        entry["task"] = allow_task
        return entry

    phases = sorted(profile["phases"])

    # Two primary agents, the same pair Claude Code gets as two slash commands: one change
    # in this checkout, or several at once with a worktree each. Both appear in the picker,
    # so the choice is made by starting the run rather than by an argument to it.
    agent = {
        "alfred": {
            "model": profile["orchestrator"],
            "mode": "primary",
            "description": "Alfred orchestrator: plans, routes and delegates. Never writes code.",
            "prompt": orchestrator_prompt(skills_root, phases, "the `alfred-worktree` agent"),
            "permission": {"task": {"*": "deny", "alfred-*": "allow"}},
            "tools": as_booleans(ORCHESTRATOR_TOOLS, allow_task=True),
        },
        "alfred-worktree": {
            "model": profile["orchestrator"],
            "mode": "primary",
            "description": "Alfred orchestrator for several changes at once, one git worktree each.",
            "prompt": fleet_prompt(skills_root, phases),
            "permission": {"task": {"*": "deny", "alfred-*": "allow"}},
            "tools": as_booleans(ORCHESTRATOR_TOOLS, allow_task=True),
        },
    }

    for phase, model in profile["phases"].items():
        if phase == "worktree":
            raise SystemExit("profile: 'worktree' is the fleet orchestrator, not a phase")
        skill_path = f"{skills_root}/{phase}/SKILL.md"
        agent[f"alfred-{phase}"] = {
            "model": model,
            "mode": "subagent",
            "hidden": True,
            "description": f"Alfred {phase} phase executor",
            "prompt": SUBAGENT_RULES.format(phase=phase, skill_path=skill_path),
            "tools": as_booleans(phase_tools(profile, phase), allow_task=False),
        }

    agent["alfred-manage"] = {
        "model": manage_model(profile),
        "mode": "subagent",
        "hidden": True,
        "description": "Alfred management: status, registry, doctor, reindex",
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

    (agents_dir / "alfred-manage.md").write_text(
        "---\n"
        "name: alfred-manage\n"
        "description: Alfred management - status, registry, doctor, reindex\n"
        f"model: {manage_model(profile).split('/', 1)[-1]}\n"
        f"tools: {', '.join(phase_tools(profile, 'alfred'))}\n"
        "---\n\n"
        + MANAGE_RULES.format(skill_path=f"{skills_root}/alfred/SKILL.md")
        + "\n"
    )
    written += 1

    commands_dir = claude_home / "commands"
    commands_dir.mkdir(parents=True, exist_ok=True)
    (commands_dir / "alfred.md").write_text(
        claude_command(profile, skills_root, sorted(profile["phases"]))
    )
    (commands_dir / "alfred-worktree.md").write_text(
        claude_worktree_command(profile, skills_root, sorted(profile["phases"]))
    )

    return written + 2


def claude_command(profile: dict, skills_root: str, phases: list[str]) -> str:
    return f"""---
description: Alfred orchestrator - plan, route and delegate a change
argument-hint: [what you want done, or: init | continue | status]
model: {profile["orchestrator"].split("/", 1)[-1]}
tools: {", ".join(ORCHESTRATOR_TOOLS)}
---

{orchestrator_prompt(skills_root, phases, "`/alfred-worktree`")}

## The request

$ARGUMENTS
"""


def claude_worktree_command(profile: dict, skills_root: str, phases: list[str]) -> str:
    """The worktree coordinator as a slash command.

    A separate command rather than a mode of /alfred, for the reason fleet_prompt gives, and
    now for a second: this one starts sessions and routes between them, where /alfred runs a
    change in the session it was asked from.
    """
    return f"""---
description: Alfred worktree coordinator - a session per change, one git worktree each
argument-hint: [branch [from base]: request, one per line, or: continue]
model: {coordinator_model(profile).split("/", 1)[-1]}
tools: {", ".join(COORDINATOR_TOOLS)}
---

{coordinator_prompt(skills_root, profile["orchestrator"].split("/", 1)[-1])}

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
            generated = opencode_config(profile, skills_root)
            merge_opencode(Path(path), generated)
            written.append(
                f"opencode: {len(generated['agent']) - 2} subagents "
                "+ alfred and alfred-worktree agents"
            )
        elif kind == "claude":
            count = claude_agents(profile, skills_root, Path(path))
            written.append(f"claude code: {count - 2} subagents + /alfred and /alfred-worktree commands")

    print("\n".join(written))
    return 0


if __name__ == "__main__":
    sys.exit(main())
