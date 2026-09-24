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
