# Workflow protocol

A workflow is a recipe: which phases exist, which routes run them in which order, where a
request enters, which of them are dispatched together, and which one closes the change.
Phases are the steps, and they are shared — a workflow reuses the library under `skills/`
or brings its own.

It is a directory holding one definition and one rules file. The definition carries the
structural facts and is machine-readable; the rules file carries the judgement about which
of that workflow's routes a request deserves, and is prose because nothing has to parse it.
That split is what everything below follows from: **structure is checked once, where the
user is looking at the workflow; judgement is carried as prose, where nothing parses it.**

Alfred ships one workflow, and everything here applies to it exactly as it applies to one
written by hand this morning. The definition key by key is in `docs/workflows.md`.

## Two scopes

A workflow is either the machine's or a repository's, and the difference is which roots it
is read from and where the commands it produces are written.

```
machine   ~/.config/alfred/workflows/<name>/          shipped, managed by the installer
          ~/.config/alfred/custom/workflows/<name>/   the user's, never touched by it
project   <repo>/.alfred/workflows/<name>/            the repository's own
```

A machine workflow is available in every repository on that machine. A repository's
workflow is available in that repository and nowhere else: registering it writes inside the
repository, per detected agent, and never outside it. A machine-level operation never
writes into a repository and a project-level one never writes outside it, which is why an
update cannot disturb a repository's workflows — it is never handed a repository to walk
into.

The installer manages the shipped root by hash like every other installed file, and never
lists the custom root in what it installs, so `install` and `update` neither read, write
nor remove anything there. That absence is the whole preservation mechanism: a custom
workflow survives every update because no update knows it exists. An absent custom root
means no custom workflows, never an error.

A name present under both machine roots registers under neither, and the report says which
roots it was found in. A custom workflow that silently replaced a shipped one would change
that command on one machine and nowhere else.

## Local over global

A repository's workflow of the same name as one of the machine's is the one that runs in
that repository. It is the rule a skill already follows under `.alfred/skills/`, applied to
a whole recipe, and for the same reason: one repository genuinely needs different steps,
and nothing outside it should have to change for that.

The override is reported, never silent. Registration states that the repository's version
replaced the machine's for that repository, because a run that quietly took the other one
is a run nobody can explain six months later.

## The phases it declares

A run dispatches the phases its workflow declares and no others. Each is resolved within
the scope of its workflow, per `skills/_shared/skill-resolver.md`: the workflow's own
phase first, then — for a repository's workflow — that repository's overrides, then the
shared library.

A phase that resolves in no root of its scope rejects the workflow at registration rather
than failing the first time a route reaches it. The failure is the same either way; what
differs is that one happens while the user is looking at the workflow, and the other
happens three phases into a change, in a repository, with far less context to explain it.

A phase nobody assigned a model to is not dispatched either. Registration names it, the
command marks it unavailable with the reason, and a route that reaches it stops and says
so rather than substituting anything for it.

## The structural facts are read once

Registration validates the definition and renders its structural facts into the command it
generates: the routes, the default route, the entry points, the groups dispatched together,
the phase that closes a change, and the subagent behind each phase. From then on the run
works from the command it was started from.

```
at registration   the definition is read, validated and rendered into the command
during a run      nothing opens a definition; the only workflow file read is the rules
```

Two consequences follow, and both are the point. A definition edited after registration has
no effect until registration runs again, which is what the operation that rescans the roots
is for. And a definition corrupted after registration changes nothing about a run in
progress, because no part of that run was ever going to read it.

The rules file is the exception, and it is prose. The orchestrator reads it before proposing
a route and stops the run naming the file when it cannot be read, per
`skills/_shared/orchestrator-protocol.md`.

## Closing a change

A change is closed by the phase its workflow declares as its closing one, whatever that
phase is called. A workflow that declares none is not registered.

Closing is the same work under every workflow:

```
remove .alfred/state/{change}.yaml, staging the removal with the closing commit
close the worktree, when the change ran in one
```

per `skills/_shared/state-contract.md` and `skills/_shared/worktree-protocol.md`. A
workflow holding no phase named `archive` closes exactly as cleanly as one that does: once
its own closing phase has run, `continue` no longer finds the change and no worktree is
left behind.

What else that phase does is its own business and differs between workflows — the record a
change leaves, a delta merged into the master specifications, a postmortem written to
memory. None of those is a condition for the change being closed; the two lines above are.
