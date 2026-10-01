# What a change leaves behind

A change running through Alfred produces a document per phase. There are up to eight of
them, they total roughly fifteen hundred lines, and they are written into the repository
the change is being made in.

```
docs/changes/login-google/
  proposal.md         refine
  research.md         research
  spec.md             spec
  design.md           design
  tasks.md            tasks
  verify-report.md    verify
  review-report.md    review
  inputs/             the material the change arrived with
```

That is correct while the change is open. Every one of those documents is how one phase
tells the next what it decided, and a subagent starting with an empty context has nowhere
else to read it from.

It stops being correct once the change closes. The requirement has been merged into the
master specifications, the code is committed, and the eight documents describe a process
that finished. By the twentieth change they are the majority of what someone sees when they
open the repository, in a repository whose subject is not Alfred.

`memory.documents` decides what happens to them.

## The three modes

| Mode | While the change is open | After it closes |
|---|---|---|
| `keep` | files | all of them stay |
| **`ephemeral`** | files | the record and the delta spec stay — **the default** |
| `pointer` | memory | the address file and the record stay |

`keep` is the original behaviour and changes nothing. It was the default until `ephemeral`
replaced it; a repository that wants every document kept sets it explicitly.

`pointer` moves the documents into the memory backend and keeps addresses for them. It
needs a backend that answers, and the pipeline stops without one.

`ephemeral` is the subject of this document.

## Ephemeral

The phases write exactly what they write today, to the same paths, in the same order.
Nothing about a phase changes. The deletion happens once, in `archive`, after the record is
written and read back.

```
during the run                        at close
docs/changes/login-google/            docs/changes/login-google/
  proposal.md      removed              record.md    the one document the change leaves
  research.md      removed              spec.md      the delta, kept
  spec.md          kept
  design.md        removed            docs/specs/    merged, as always
  tasks.md         removed
  verify-report.md removed            .alfred/state/login-google.yaml   removed
  review-report.md removed
  inputs/          removed
```

The volume problem is retention, not writing. Writing the documents is what makes the
pipeline work; keeping them after the question they answered is settled is what fills the
repository.

## What is never removed

```
paths.master_specs         what the system does, merged from every delta
docs/architecture.md       what the repository is
docs/code_conventions.md   how it is built
the delta spec             artifacts.keep_delta_spec, on by default
the record                 always
```

The master specifications are the reason the deltas exist, and the delta is how a merged
requirement is reviewed. Keeping both is a decision: deltas are how a change is read at the
time, master specs are how the system is understood afterwards, and dropping either leaves
an unreadable diff or a document that silently goes stale.

## The record

One document, written by `archive` from `templates/docs/record.md`, to a location
`paths.change_records` decides.

```yaml
paths:
  change_records: docs/changes/
```

It answers, for someone arriving months later from a master specification: what the system
does now, why, **how it was decided**, what was required, and how it ended.

`How it was decided` is the section the mode depends on. It carries the approach taken and
the alternatives rejected with the reason for each, lifted out of the design document that
is about to be deleted. It is the only part of the design that survives, and without it the
next change reopens a settled decision because nothing recorded that it was settled.

## What it costs

With a memory backend, the working documents were indexed as each phase completed, and
memory holds them. With `backend: none`, there is no second copy and they are gone.

They are removed from the working tree before the closing commit is staged, so git never
holds them either. This is deliberate: committing them and then deleting them would mean two
commits per change, and a history where every change appears twice.

So the trade is explicit. What is discarded is the reasoning in progress. What is kept is
the conclusion, in the record, and the requirement, in the specifications. If a repository
wants the reasoning too, that is what `keep` is for.

## Configuration

```yaml
paths:
  changes: docs/changes/
  change_records: docs/changes/

artifacts:
  retain: final_only
  keep_delta_spec: true
  state_on_completion: delete

memory:
  documents: ephemeral
```

| Key | Values | Meaning |
|---|---|---|
| `artifacts.retain` | `final_only`, `all` | whether the working documents are removed at close |
| `artifacts.keep_delta_spec` | `true`, `false` | whether the delta survives beside the record |
| `artifacts.state_on_completion` | `delete`, `keep` | whether the state file is removed when the change closes |
| `paths.change_records` | a path | where the record goes |

## The state file

`.alfred/state/{change}.yaml` exists so `continue` can resume a change. A change that closed
has nothing to resume, and the file stops describing anything: every phase reads
`completed`.

`archive` removes it, and stages the removal with the closing commit so it leaves the
working tree and the repository together. A file deleted on disk but left tracked comes back
on the next checkout, and `continue` finds a change that finished months ago.

It is kept whenever the change did not close: a phase failed, `verify` failed, `review`
blocked, the change was abandoned, or `archive` closed work that was explicitly incomplete.
That last one matters — open work is work somebody may come back to, and a record reading
"three tasks unfinished" beside no state leaves nothing able to resume them.

## Not the same question as `artifacts.committed`

`artifacts.committed` decides whether Alfred's documents are committed at all.
`memory.documents` decides which of them survive the change. They are independent, and
`docs/artifacts.md` has the table of what the four combinations leave a reader.

## Repositories that predate the default

A repository initialised before `ephemeral` existed carries `memory.documents: keep` in its
own `.alfred/config.yaml`, and rule 2 says the repository's configuration wins. It keeps
behaving exactly as it did, and `update` changes nothing about it.

The new default applies to repositories initialised from now on, and to a repository whose
configuration does not name the key at all. Nothing is rewritten on anyone's behalf: a
setting that decides whether documents are deleted is not one to change under a user who did
not ask.

`alfred status` reports the mode in force and where it came from, so a repository still on
`keep` says so rather than looking like one that chose it.

## Switching

Nothing has to be migrated. The modes differ in what `archive` does at close, not in where
phases write, so a repository can change `memory.documents` between changes and the change
in flight finishes under whichever mode is set when it closes.

Going from `ephemeral` back to `keep` does not restore documents already removed. They are
in memory where a backend was configured.
