# Workflows

A workflow is a recipe: which phases exist, which routes run them in which order, where a
request enters, which phases are dispatched together, and which one closes the change.
Alfred ships one, `sdd`, and it is spec-driven development — the pipeline most of this
documentation describes. It is not privileged. A workflow you write this morning is read,
validated and turned into a command by exactly the same code.

Each workflow becomes one command per agent. `/alfred-sdd` runs `sdd`; `/alfred-billing`
runs a workflow called `billing`; `/alfred` runs whichever one the machine's default names.

A workflow is a **directory** holding two files:

```
billing/
  workflow.json   the structural facts: phases, routes, entry points, groups, closing phase
  rules.md        the judgement: which route a request deserves, and why
  skills/         optional, one SKILL.md per phase the shared library does not have
```

That split is the whole design. The definition is machine-readable and is read once, at
registration; the rules file is prose and is read by the orchestrator during a run, because
nothing has to parse it. Structure is checked where you are looking at the workflow;
judgement travels as prose.

## The two scopes

```
machine   ~/.config/alfred/workflows/<name>/          shipped with Alfred, managed by hash
          ~/.config/alfred/custom/workflows/<name>/   yours, never touched by an update
project   <repo>/.alfred/workflows/<name>/            one repository's own
```

A machine workflow is available in every repository on that machine. A repository's
workflow is available in that repository and nowhere else: registering it writes commands
and agents **inside** the repository, per detected agent, and never outside it.

The custom root is the preservation mechanism and it works by absence. The installer does
not list it among the files it installs, so `install` and `update` never read, write or
remove anything there, and a workflow you wrote survives every update because no update
knows it exists. The directory does not exist until something writes the first workflow
into it.

A name present under **both** machine roots registers under neither, and the report says
which roots it was found in. A custom workflow silently replacing a shipped one would
change that command on one machine and nowhere else.

### When a repository overrides a machine workflow

A repository defining its own `billing` is the one that runs there, and the machine's
`billing` still runs in every other repository. The override is reported, never silent.

The repository's copy is reached as `/alfred-billing-local`, and its own phases become
`alfred-billing-local-<phase>`:

```
machine-scope billing      ->  /alfred-billing
this repository's billing  ->  /alfred-billing-local
```

The distinguishing name exists because whether an agent resolves a project-scope command
before a machine-scope one of the same name was never measured, on either Claude Code or
OpenCode. A distinct name is correct whichever way that resolves. A repository workflow
whose name exists at no machine root keeps the plain `/alfred-<name>`: there is no second
command, so precedence never arises. Registration prints which name to use, and that line
is the only thing standing between a user and typing `/alfred-billing` in a repository that
overrides `billing` and silently getting the machine's.

## `workflow.json`

```json
{
  "name": "billing",
  "title": "Billing follow-up",
  "description": "Draft, invoice, chase and close",
  "rules": "rules.md",
  "phases": [
    {"name": "draft", "model": "anthropic/claude-haiku-4-5"},
    {"name": "invoice", "model": "anthropic/claude-opus-5", "tools": ["Read", "Write", "WebFetch"]},
    {"name": "chase", "model": "anthropic/claude-haiku-4-5"},
    {"name": "review"}
  ],
  "routes": {
    "express": ["invoice", "review"],
    "standard": ["draft", "invoice", "chase", "review"]
  },
  "default_route": "standard",
  "entry_points": {"default": "draft"},
  "parallel": [["invoice", "chase"]],
  "closes": "review"
}
```

| Key | Required | Meaning |
|---|---|---|
| `name` | yes | `^[a-z][a-z0-9-]*$`, equal to the directory name, and not a reserved name |
| `title` | yes | the command's one-line title |
| `description` | yes | what the workflow is for, shown when listing the workflows available |
| `rules` | yes | the rules file, relative to the directory |
| `phases` | yes | every phase the workflow may run, one object each |
| `phases[].name` | yes | `^[a-z][a-z0-9-]*$`; the phase, and never `init`, `explore` or `worktree` |
| `phases[].model` | conditional | `^[A-Za-z0-9][A-Za-z0-9._:/-]*$`; required when the phase does not resolve to the shared library, and refused when it does |
| `phases[].tools` | no | each entry `^[A-Za-z][A-Za-z0-9_-]*$`; an own phase's tool set, absent means the default set, and refused when the phase resolves to the shared library |
| `phases[].workflow` | no | reserved for a step that triggers another workflow; refused today |
| `routes` | yes | at least one; each name `^[a-z][a-z0-9-]*$`, each an ordered list of declared phases |
| `default_route` | yes | one of the declared routes |
| `entry_points` | yes | at least one; each value a declared phase |
| `parallel` | yes | possibly empty; each group two or more declared phases |
| `closes` | yes | the declared phase that closes a change |

It is JSON, decoded into a typed struct with unknown keys rejected, so a misspelled key is
refused by name and a syntax error is reported with its position. There is no format
version field: an older Alfred already refuses a key it does not know, with a clearer
message than a version number would carry.

Reserved names, which a workflow may not be called: every shared phase name, plus `alfred`,
`manage`, `worktree`, `init`, `explore`, `add-workflow` and `workflows-scanner`.

### Four values held to a shape

A phase's name, its model, each of its tools and each route's name are refused outright
when they do not match the patterns above:

```
billing  rejected: declares a phase named "../quote", which does not match
         ^[a-z][a-z0-9-]*$
billing  rejected: phase "quote" declares a model "sonnet 4", which does not
         match ^[A-Za-z0-9][A-Za-z0-9._:/-]*$
billing  rejected: phase "quote" declares a tool "Bash(git diff:*)", which does
         not match ^[A-Za-z][A-Za-z0-9_-]*$
billing  rejected: declares a route named "express lane", which does not match
         ^[a-z][a-z0-9-]*$
```

A phase name is a filename component before it is anything else: resolution joins it onto
a skill root and generation joins it onto an agent directory, so a name carrying `..`
leaves both. A model and a tool name are each written onto one line of a generated Claude
Code subagent's frontmatter, where a character that ends or splits the line writes a key
Alfred never wrote. A route name is addressed by the user, printed in the registration
report and rendered into the generated command, so holding it more loosely than the
workflow it belongs to buys nothing. A repository's workflows arrive with a `git clone`
and are registered by `init` or the scanner without anyone reading them, so all four
answer to a shape rather than being taken at their word.

The workflow's name, a phase's name and a route's name are one pattern. They are the three
names a definition carries, and they follow one rule you learn once.

Claude Code's scoped tool form, `Bash(git diff:*)`, is refused by the tool pattern. A
phase that needs a narrower tool than `Bash` declares the bare tool and says the rest in
its skill.

`phases[].workflow` is a **defined** key that validation refuses:

```
billing  rejected: phase "quote" names a workflow; a step that references a
         workflow is not supported yet
```

Defined rather than unknown is the point. It reserves the shape a sub-workflow step will
take, so a definition written today is unaffected when the key is eventually honoured.

### Phases, and where one comes from

A phase is resolved within the scope of its workflow:

```
the workflow's own skills/<phase>/SKILL.md       first
.alfred/skills/<phase>/SKILL.md                  for a repository's workflow only
~/.config/alfred/skills/<phase>/SKILL.md         the shared library
```

A workflow's own `review` is a different phase from the shared `review` and never collides
with it: shared phases are the subagent `alfred-<phase>`, a workflow's own are
`alfred-<workflow>-<phase>`.

A machine workflow does not get the middle root. It is available in every repository, so
resolving one of its phases against one repository's overrides would register a workflow
that works there and is missing a phase everywhere else. A phase only a repository can
provide belongs to a repository-scope workflow, and the report says so.

A phase that resolves in no root of its scope **rejects the workflow at registration**. The
failure is the same either way; what differs is that this one happens while you are looking
at the workflow, rather than three phases into a change.

### Models

```
a phase from the shared library        its entry in ~/.config/alfred/profile.json
a phase from the workflow's own skills/   its "model" in workflow.json
a phase from .alfred/skills/              the same
```

A workflow's own phase never falls back to the profile — an own `review` must not silently
inherit the shared `review`'s assignment — and nothing ever falls back to the
orchestrator's model.

A shared phase's model is chosen once, at installation, in `~/.config/alfred/profile.json`,
and that is the only place it is written. Its tool set comes from the installation too:
Alfred's base set for that phase, plus whatever `profile.json` added to it. A definition
that declares a `model` or a `tools` on a phase resolving to the shared library is
**refused** rather than silently overruled, and the workflow is rejected:

```
billing  rejected: phase "review" declares a model and resolves to the shared library,
         where the model is the installation's assignment in profile.json
billing  rejected: phase "review" declares a tool set and resolves to the shared library,
         where the tool set is the installation's assignment
```

It is one rule about two keys: whichever side created the phase is the side that says what
it runs on and what it runs with. Refusing rather than reporting is deliberate. A
declaration the user wrote that has no effect, mentioned once in a report nobody re-reads,
is the silent discard under another name. Assign a shared phase's model and its extra tools
through the profile, where they apply to every workflow that uses that phase, or bring the
phase as the workflow's own under its `skills/` and declare both there.

A phase with a model from neither side gets no subagent. The workflow still registers, and
still runs every route that does not reach that phase; registration names the phase under
*without a model*, the command marks it unavailable with the reason, and a run that reaches
it stops and says which phase and which assignment is missing. No model is chosen on your
behalf.

## `rules.md`

Prose, read by the orchestrator before it proposes a route, and nothing parses it. It
carries everything the definition cannot:

- which signals each route matches, and the route proposed when none of them does
- how a route changes mid-run, and what the user is told when it does
- which phase receives external material for which kind of request
- the branch a phase takes when it has a judgement to make, and where each answer leads

It carries nothing the definition already states. A route list in the rules file is a
second authority that nothing keeps in step.

A rules file that cannot be read **stops the run**, naming the file and its path. No route
is applied in its place, because the alternative is Alfred choosing one without the only
document that says how.

## Adding one

```
/alfred-add-workflow
```

The interview settles the name, the phases, the routes, the default route, the entry
points, the parallel groups, the closing phase and the rules, asks for the model of every
phase the shared library does not provide, shows you the assembled workflow and waits.
Accept it and the directory is written under `~/.config/alfred/custom/workflows/<name>/`
from `templates/workflow/`, one skill per new phase, and registration runs and names the
new command. Decline it and nothing is written and nothing is registered.

A model question left unanswered is not a blocker: the phase is shown as having no model
and is written that way, and the rules above apply.

Writing the directory by hand is equally supported — the interview creates no state the
scan does not re-derive. Create the two files and run registration.

## Registration

Registration scans the roots of one scope, validates every definition it finds, and writes
one command per workflow with that workflow's structural facts rendered into it, plus the
subagents its phases need.

| Scope | Triggered by |
|---|---|
| machine | `./install.sh install`, `update`, `models`, `./install.sh workflows`, `bin/register.sh`, `/alfred-workflows-scanner`, the end of `/alfred-add-workflow` |
| project | `init`, at the end of setting the repository up; `/alfred-workflows-scanner` run from the repository; the `workflows` operation |

`install` and `update` never trigger project scope. A machine-level operation that walked
into repositories would have to find them first, and an update must not be able to touch
a repository's own workflows.

From inside an agent, on a machine with no clone:

```
bin/register.sh                    machine scope: scan, validate, regenerate, report
bin/register.sh --dry-run          the same report, writing nothing
bin/register.sh --check            the workflows with no command; exits non-zero if any
bin/register.sh --project <repo>   the same three, for one repository's own workflows
```

It shells to the helper installed at `~/.config/alfred/bin/alfred` and needs no Go
toolchain and no source tree.

### The report

```
workflows
  sdd         machine/shipped  10 phases  routes: direct, pipeline, full
  billing     project          4 phases  routes: express, standard
              overrides the machine's billing in this repository; use /alfred-billing-local here, /alfred-billing for the machine's
  fulfilment  machine/custom   rejected: route "express" names phase "pickng", which it
                               does not declare

claude code  (project: /Users/x/code/storefront/.claude)
  added      /alfred-billing-local, alfred-billing-local-draft
  unchanged  1 command, 2 agents

excluded     3 paths recorded in .git/info/exclude

without a model
  billing/review  the workflow's own review declares no model and will not be dispatched
```

One rejected workflow never blocks the valid ones: it is reported with the reason and every
other workflow registers. A run over a tree that changed in no way writes nothing and says
so.

A workflow removed from its root loses its command on the next registration, and only
because registration recorded writing it. Removal follows that record, never a name
pattern, so a file you wrote and happened to call `alfred-notes` is reported when it
collides and is never deleted.

### What a run does with all of this

Nothing parses a definition during a run. The routes, the entry points, the groups
dispatched together, the closing phase and the subagent behind each phase were read and
validated at registration and are in the command the run started from. Two consequences,
and both are deliberate: a definition edited after registration takes effect when
registration runs again, and a definition corrupted after registration changes nothing
about a run in progress.

The only workflow file a run opens is the rules file.

## Closing a change

A change is closed by the phase its workflow declares as `closes`, whatever it is called. A
workflow that declares none is not registered. Closing is the same work under every
workflow — the state file is removed and staged with the closing commit, and the worktree
is closed when the change ran in one — and what else that phase does is the workflow's own
business. A workflow with no phase named `archive` closes exactly as cleanly as `sdd` does.

## `sdd`

The shipped workflow. Ten phases — `refine`, `research`, `spec`, `diagnose`, `design`,
`tasks`, `apply`, `verify`, `review`, `archive` — three routes, `direct`, `pipeline` and
`full`, a feature entering at `refine` and a defect at `diagnose`, `verify` and `review` as
the one group dispatched together, and `archive` as the closing phase. It declares no phase
of its own, so it carries no model assignment and `profile.json` stays the only place a
shared phase's model is written.

Everything `sdd` judges rather than declares is in `workflows/sdd/rules.md`: which route a
request deserves, which signals say so, and where `diagnose` goes next. If you want
spec-driven development to behave differently in one repository, copy the directory to
`.alfred/workflows/sdd/` and edit it there.
