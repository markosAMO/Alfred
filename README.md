# Alfred

A spec-driven workflow for building software with AI agents.

Alfred is not a program. It is a package of instructions that configures the agents you
already use — Claude Code, OpenCode, OpenClaw and others — so that software work follows the
same pipeline every time, in every repository.

Almost all of it is Markdown. The only code is the installer.

## What it does

A request enters, and a route is chosen and proposed before anything happens.

```
behaviour unchanged      direct     apply · verify · review
behaviour changes        pipeline   spec · design · tasks · apply · verify · review · archive
request underspecified   full       refine · research · then the pipeline
defect reported          diagnose   then spec or design
```

Each phase runs as a subagent with an empty context, receiving paths rather than documents.
The orchestrator plans and delegates; it never writes code and never reads a specification.

Specifications live in the repository, under version control. Every change is written as a
delta, and merging that delta back into the master specifications is a step in the work
rather than something to remember afterwards. Documentation stops going stale because
updating it is not optional.

## Phases

| Phase | Question |
|---|---|
| `init` | how is this repository set up |
| `explore` | what does the existing code already say |
| `refine` | what is actually being asked for |
| `research` | what is not known yet |
| `spec` | what must the system do |
| `diagnose` | why does this fail |
| `design` | how will this codebase do it |
| `tasks` | what are the units of work, and in what order |
| `apply` | build it |
| `verify` | does it satisfy the specification |
| `review` | is the code sound |
| `archive` | fold it in, and commit |

`verify` and `review` are separate because they answer different questions, and `review`
runs in a context that never saw the code being written.

## Agnostic by design

Nothing in a skill names a concrete tool.

| Layer | Contract | Backends |
|---|---|---|
| Memory | `memory/CONTRACT.md` | Engram, a shared Postgres, or none |
| Notification | `notify/CONTRACT.md` | terminal, OpenClaw (WhatsApp, Telegram) |
| Task tracker | `tracker/CONTRACT.md` | none, Jira, GitHub Issues, Linear |

Every layer degrades rather than failing. With no memory backend the pipeline still runs,
because the artifacts are files and memory only makes them searchable. Switching backend is
a reindex, with nothing to export.

The same applies to models: each phase runs on its own, assigned through named profiles, so
bounded execution can run on a local model while judgement stays hosted.

And to stacks. Code conventions live in the repository being worked on, never here, so one
Alfred serves a Rails API and a React front end without changing.

## Multiple repositories

A change spanning several repositories is planned once at workspace level and executed
independently in each, with each repository holding a complete description of its own part
and a link back to the system-level requirement. Cross-repository dependencies are declared
and respected.

## Installation

```bash
git clone https://github.com/markosAMO/Alfred.git
cd Alfred
./install.sh install
```

That registers the orchestrator with the agents on the machine and installs the skills
globally: the `/alfred` command in Claude Code, a primary agent in OpenCode.

Each repository is then set up from the orchestrator itself — `/alfred init` — with no
global `alfred` command and nothing to copy in by hand.

See [docs/installation.md](docs/installation.md) for the installer commands, the pipeline
commands, and troubleshooting.

## Documentation

| | |
|---|---|
| [AGENTS.md](AGENTS.md) | repository map, pipeline, hard rules |
| [docs/installation.md](docs/installation.md) | the two installation levels |
| [docs/models.md](docs/models.md) | model assignment and profiles |
| `skills/_shared/` | the protocols every phase relies on |

## Acknowledgements

The spec-driven approach, delta specifications and the phase structure are inspired by
[agent-teams-lite](https://github.com/Gentleman-Programming/agent-teams-lite)

## License

MIT
