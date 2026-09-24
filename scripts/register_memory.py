"""Register the configured memory backend with every agent, and keep it whole.

Two layers have to agree for a phase to reach a memory tool: the agent declares the tool,
and the MCP server exposes it. `generate_agents.py` owns the first. This owns the second.

They fail differently and only one of them is visible. An agent that declares a tool the
server does not expose sees no tool at all, which from inside a phase is indistinguishable
from a backend that cannot do the thing - the failure this whole arrangement exists to stop.
So the registration is written by the installer rather than pasted from a document, and a
narrowed one already on disk is repaired rather than reported.

Usage:
    register_memory.py check <alfred_home> [target ...]
    register_memory.py apply <alfred_home> [target ...]

Targets are `claude=<home>` or `opencode=<config.json>`, as generate_agents.py takes them.
Exit code is the number of problems found; `check` never writes.
"""
import json
import shutil
import sys
from pathlib import Path

# Every tool the backend exposes, never a subset. A subset is a copy of the backend's
# surface that nothing keeps in step, and it degrades silently as the backend grows.
PROFILE = "all"


def configured_backend(alfred_home: Path) -> str:
    """The `memory.backend` value, read without a YAML parser."""
    config = alfred_home / "alfred.config.yaml"
    if not config.is_file():
        return ""
    in_memory = False
    for line in config.read_text().splitlines():
        if line.startswith("memory:"):
            in_memory = True
            continue
        if in_memory:
            if line and not line.startswith((" ", "\t")):
                break
            key, _, value = line.strip().partition(":")
            if key == "backend":
                return value.strip()
    return ""


def binary(name: str) -> str:
    """The absolute path, because an agent does not inherit the shell's PATH."""
    return shutil.which(name) or name


# The two agents spell a stdio server differently: Claude Code splits command and args,
# OpenCode carries both in one list. Same registration, two shapes.
def wanted(kind: str, name: str) -> dict:
    if kind == "opencode":
        return {"type": "local",
                "command": [binary(name), "mcp", f"--tools={PROFILE}"],
                "enabled": True}
    return {"type": "stdio", "command": binary(name),
            "args": ["mcp", f"--tools={PROFILE}"], "env": {}}


def invocation(entry: dict) -> list:
    """The argument list, whichever shape the agent writes it in."""
    command, args = entry.get("command"), entry.get("args") or []
    if isinstance(command, list):
        return command
    return [command, *args] if command else list(args)


def diagnose(entry) -> str:
    """'missing', 'narrowed', or '' when the registration is whole."""
    if not isinstance(entry, dict):
        return "missing"
    parts = [p for p in invocation(entry) if isinstance(p, str)]
    if "mcp" not in parts:
        return "missing"
    tools = next((p.split("=", 1)[1] for p in parts if p.startswith("--tools=")), None)
    # No --tools at all is the backend's own default, which is every tool.
    return "" if tools is None or tools.strip() == PROFILE else "narrowed"


def load(path: Path) -> dict:
    try:
        return json.loads(path.read_text())
    except Exception:
        return {}


def save(path: Path, data: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n")


def handle(path: Path, container: str, container_kind: str, name: str, apply: bool) -> str:
    """Diagnose one agent's config, repairing it when asked. Returns the status."""
    data = load(path)
    servers = data.get(container)
    if not isinstance(servers, dict):
        servers = {}
    status = diagnose(servers.get(name))
    if status and apply:
        servers[name] = wanted(container_kind, name)
        data[container] = servers
        save(path, data)
    return status


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__.strip(), file=sys.stderr)
        return 2
    mode, home, targets = sys.argv[1], Path(sys.argv[2]).expanduser(), sys.argv[3:]
    apply = mode == "apply"
    if mode not in ("check", "apply"):
        print(f"unknown mode: {mode}", file=sys.stderr)
        return 2

    backend = configured_backend(home)
    if backend in ("", "none"):
        print(f"memory backend is {backend or 'unset'}; nothing to register")
        return 0
    if not shutil.which(backend):
        # Configured but absent: report it, do not invent a registration for a binary that
        # is not there. `memory.required` decides whether that is fatal, at run time.
        print(f"FAIL  {backend} is the configured backend and is not on PATH")
        print(f"      install it, or set memory.backend: none in {home}/alfred.config.yaml")
        return 1

    problems = 0
    for target in targets:
        kind, _, where = target.partition("=")
        if kind == "claude":
            path, container = Path(where).expanduser() / ".claude.json", "mcpServers"
        elif kind == "opencode":
            path, container = Path(where).expanduser(), "mcp"
        else:
            continue

        status = handle(path, container, kind, backend, apply)
        label = f"{kind}: {backend}"
        if not status:
            print(f"ok    {label} registered with every tool")
        elif apply:
            print(f"fixed {label} was {status}, now --tools={PROFILE}")
        else:
            problems += 1
            print(f"FAIL  {label} registration is {status}")
            print(f"      run: {sys.argv[0].rsplit('/', 1)[-1]} apply, or ./install.sh update")
    return problems


if __name__ == "__main__":
    sys.exit(main())
