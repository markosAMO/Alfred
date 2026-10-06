# Where Alfred's documents live

Alfred writes documents: an architecture, a conventions file, master specifications, and a
directory per change. `artifacts.committed` decides whether they are committed to the
repository they describe.

```yaml
artifacts:
  committed: true
  local_exclude: .git/info/exclude
```

## committed: true

The default, and the one to prefer. The documents are versioned with the code they
describe, so a specification and the behaviour it specifies move together, a reviewer sees
why a change says what it says, and a second person cloning the repository gets the whole
picture without being told anything.

It is also what makes the documents safe. Git is holding them; nothing else has to.

## committed: false

For a repository that will not take them. A team that has not adopted Alfred, a review
process that would reject the directory, a checkout that is not yours to reshape. Alfred
runs there unchanged — the pipeline, the phases and the documents are the same — and the
documents stay out of git deliberately.

They are excluded through `artifacts.local_exclude`, which defaults to `.git/info/exclude`.
That file is per-clone and is not itself tracked, so keeping Alfred out of the repository
does not require a commit to the repository. `.gitignore` would be a change to the very
thing the mode exists to avoid changing.

Every pattern written there is anchored to the repository root, so it excludes the
repository's own `.alfred/` and not a directory of that name at any depth under it.
Unanchored, it hides a vendored dependency's `.alfred/` or a test fixture under
`docs/specs/` along with Alfred's, and a path git is ignoring cannot be staged and never
appears in `git status`.

git infers the anchor for any pattern carrying a directory in it, which is every concrete
path registration writes but one, so the anchor is written where git would not infer it:
`init` writes its four with a leading slash — `/.alfred/`, `/docs/specs/` — and
registration adds one to `opencode.json`, the only path it writes with no directory in it.

`init` writes the exclusions when it sets the mode. Nothing else in the pipeline behaves
differently because of it, with three exceptions, all of them about who is holding the
files.

### A worktree has to be given them

A git worktree is created from a commit. Documents that were never committed are not in it
and never will be, so `worktree.sh open` copies them from the main checkout and reports
`artifacts: copied`. Under `committed: true` it reports `artifacts: tracked` and copies
nothing, because the branch already carries them.

### Closing a worktree has to refuse

`close` discards untracked files. That is right for a copied `.env` and wrong for the
specification of the change that just finished, which in this mode is also untracked.

So `close` compares the worktree's documents against the main checkout, and refuses when it
holds any the main checkout does not have byte for byte. `archive` copies them back before
asking for the close; the refusal means it did not, and it names the files rather than
guessing.

### The commit contains only code

`archive` stages the code and leaves the documents where they are. The commit message still
names the change, and the change directory is still the record of it — in the main checkout
rather than in the history.

## What you give up

The documents stop being versioned. There is no diff of a specification over time, no way
to see what a requirement said when a commit was made, and no copy on anyone else's
machine. Two copies exist: the main checkout, and the memory backend if one is configured.

This is the mode where a memory backend stops being an optimisation. Under
`committed: true` memory is a searchable copy of files git is holding, and `reindex`
rebuilds it from them at any time. Under `committed: false` it is the second copy.

Back up the main checkout's Alfred directories, or run with a memory backend, or both. The
risk is not hypothetical: a worktree removed by hand, a directory cleaned up, a machine
replaced, and the documents are gone with no history to recover them from.

## What registration writes into a repository

`artifacts.committed` governs one more thing, and it is not a document. A repository that
defines a workflow of its own under `.alfred/workflows/` gets commands and agents generated
**inside** it, one set per detected agent:

```
.claude/commands/alfred-<workflow>.md    the command for that workflow
.claude/agents/alfred-<workflow>-*.md    a subagent per phase the workflow brings itself
opencode.json                            the same agents, merged into what is already there
.alfred/generated.json                   what was written, so a later run can remove it
```

A repository that defines no workflow of its own has none of this written into it, under
either setting, and keeps every workflow the machine provides.

Under `committed: false`, every path registration wrote is appended to
`artifacts.local_exclude`, never to `.gitignore`, by the same rule and for the same reason
as the documents. Under `committed: true` they are ordinary files and nothing is appended,
with one exception: `.alfred/generated.json` is excluded in **both** modes, exactly as
`.alfred/skill-registry.md` is. It records the hashes of files generated from one machine's
model profile, so it is true of that machine and wrong on every other.

### Committing them has a cost

A generated command carries the model identifier chosen on the machine that generated it
and absolute paths into that machine's installation. Committed, those are wrong for every
other clone: a colleague who checks the repository out gets a command naming a model they
may not have and skill paths that resolve to nothing.

Nothing can make them machine-neutral while a command carries a model, so this is a
trade-off rather than a defect to be fixed. `alfred doctor` reports a committed
project-level command whose skill paths do not exist on this machine, which is what turns
it from a silent wrong answer into a thing somebody can act on: re-register, and the files
are rewritten for the machine they are on.

If the repository's own workflow is something the team shares, commit the definition —
`.alfred/workflows/<name>/workflow.json` and its `rules.md` are machine-neutral and are the
real artefact — and let each clone generate its own commands by running registration.

## Which documents, and for how long

This setting decides **whether** Alfred's documents are committed. `memory.documents`
decides **which of them still exist** once a change closes. They are independent, and both
have to be read to know what a repository ends up holding.

```
artifacts.committed    does git hold them
memory.documents       do they survive the change
```

Under `memory.documents: ephemeral`, the default, `archive` removes the working documents at
close and leaves the record and the delta specification. See `docs/ephemeral.md`.

The two combine into four repositories:

| `committed` | `documents` | What a reader finds |
|---|---|---|
| `true` | `ephemeral` | the record and the delta, in git, with history |
| `true` | `keep` | every document of every change, in git |
| `false` | `ephemeral` | the record and the delta, on one machine, untracked |
| `false` | `keep` | every document, on one machine, untracked |

The third row is the one to be careful with. Nothing about it is broken, and it is the right
answer for a repository that will not take Alfred's documents at all — but it stacks the two
ways of not keeping something, and what is left of a change is a single untracked file in
one person's checkout. Back that checkout up, or run a memory backend, or both. The warning
under `committed: false` above applies twice over here.

## An accepted loss

`ephemeral` against `backend: none` discards the working documents with no second copy
anywhere. They are removed before the closing commit is staged, so git never held them
either.

This is a decision, taken deliberately, and it is recorded here rather than left to be
discovered:

- What a later reader needs is lifted into the record **first**, and the record is read back
  before anything is deleted. A failed read-back deletes nothing.
- The delta specification and the master specifications stay as files, so what the change
  required is never in question.
- What is discarded is the reasoning in progress — the proposal, the research, the design
  narrative, the task breakdown and the two reports.
- The alternative was committing the documents and then deleting them, which means two
  commits per change and every change appearing twice in the history. That was judged worse
  than the loss.

`init` states it when the mode is chosen against no backend, and `alfred doctor` reports the
pair as a standing condition. A repository that wants the reasoning kept has `keep`, and a
repository that wants it kept and searchable but off disk has `pointer`.

## The project's own documentation

Separate question, separate setting. `artifacts.committed` is about Alfred's documents;
`git.documentation` is about the project's technical documentation, the files a reader of
the repository consults.

```yaml
git:
  documentation: with_change
```

`with_change`, the default, updates it as part of the change and commits it with the rest.
It should be the answer: documentation that is updated in a separate pass is documentation
that goes stale between passes, and the risk of `separate` is that the pass never comes.

`separate` is for a repository whose review process wants documentation on its own. It is
read by `tasks`, which then creates no documentation task — which is the reason the setting
exists at all. Deciding it at the end instead means a task was dispatched, written and
reviewed before being dropped, at the price of a subagent per change.

Merging the delta into the master specifications is not affected by either value. That is
how the specifications stay true, and it happens on every change.

## Changing your mind

Switching to `committed: true` is `git add` on the paths in `paths`, plus removing the
lines `init` wrote to the local exclude file. Nothing in Alfred has to be migrated: the
files are already where the tracked mode expects them.

The local exclude file is append-only: registration adds a line for every path it writes
and never takes one back, so a path that leaves the generated set keeps its line, and a
repository flipping `artifacts.committed` from `false` to `true` has to remove the lines
registration wrote by hand as well as the lines `init` wrote. Nothing in Alfred removes
them for you.

Leave `.alfred/generated.json` excluded, and decide separately about any generated command
and agent: they are the one part of what Alfred writes that is specific to the machine
that wrote it, and the section above is the trade-off.
