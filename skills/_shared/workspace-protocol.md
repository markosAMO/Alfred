# Workspace protocol

Alfred runs at two levels. A change touching several repositories is planned once at
workspace level and executed independently in each repository.

```
workspace/            .alfred/     system-level planning
├── front/            .alfred/     its own pipeline
├── api-billing/      .alfred/     its own pipeline
└── api-users/        .alfred/     its own pipeline
```

A repository with `.alfred/` does not need to know whether a workspace exists above it.
Single-repository work is the same pipeline with the workspace level absent.

## Prerequisite

Every repository in a workspace is initialised before the workspace is. The workspace
neither initialises nor explores them: it reads the `docs/architecture.md` and
`docs/code_conventions.md` that each repository's own `init` already produced.

That information was derived once and confirmed by the user. Deriving it again at workspace
level would cost more, produce a second description of the same system, and put the two out
of step the first time one changed.

A repository added later is initialised on its own and then listed in the workspace
configuration.

## Division of responsibility

| Level | Phases | Question answered |
|---|---|---|
| Workspace | `refine`, `research`, `spec`, `design`, `tasks` | what changes in the system, and which repository does what |
| Repository | `design`, `tasks`, `apply`, `verify`, `review`, `archive` | how this repository implements its part |

Workspace `design` allocates work to repositories. Repository `design` decides the
implementation inside one repository. Both exist because they answer different questions.

## Distribution

When workspace `tasks` runs, it writes each repository's slice into that repository.

```
workspace/docs/changes/payment-flow/
  spec.md        the system requirement
  design.md      which repository does what
  tasks.md       the allocation

api-billing/docs/changes/payment-flow/
  spec.md        only this repository's part
  design.md
  tasks.md
```

Each repository holds a complete, self-contained description of its own work. Cloned on
its own, it still reads as a coherent change.

## Traceability

Links run in both directions.

```yaml
# api-billing/docs/changes/payment-flow/spec.md
parent: workspace:payment-flow

# workspace/docs/changes/payment-flow/tasks.md
repos: [api-billing, api-users, front]
```

Without `parent`, a repository opened months later shows a change with no explanation of
the requirement it served. Without `repos`, the workspace cannot tell whether a change is
finished.

## Cross-repository dependencies

`tasks` declares them. Alfred respects the order.

```yaml
repos:
  api-billing:
    depends_on: []
  api-users:
    depends_on: []
  front:
    depends_on: [api-billing]
```

A repository whose dependencies are not `completed` is not dispatched. `front` is not
implemented against an endpoint that does not exist yet.

Repositories with no dependency between them may run at the same time.

## Completion

Each repository commits and opens its own pull request, following its own conventions.

The workspace change is complete when every repository in `repos` has passed `verify` and
`review`. Workspace `archive` runs only then, and merges the system-level delta into the
workspace master specs.

A repository that fails leaves the workspace change open, and `status` reports which
repository is holding it.
