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

## Changing your mind

Switching to `committed: true` is `git add` on the paths in `paths`, plus removing the
lines `init` wrote to the local exclude file. Nothing in Alfred has to be migrated: the
files are already where the tracked mode expects them.
