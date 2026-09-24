---
name: archive
mode: auto
skippable: false
reads: [spec, diagnosis, verify_report, review_report, master_specs]
writes: [master_specs, record, postmortem, commit]
document: {paths.change_records}/{change}/record.md
next: []
---

# archive

Close the change: fold what was learned into the places that outlive it, then commit.

This is the phase that keeps the documentation from going stale, because updating it is a
step in the work rather than something to remember afterwards.

## Preconditions

```
verify   passed
review   no blocking findings
```

Neither is inferred from the absence of an error. Both reports are read and their results
checked. A change archived without them merges a delta describing behaviour nobody
confirmed exists.

## Final state, not the last snapshot

The reports this phase reads are snapshots. `verify` ran at a moment; commits may have
landed since. Archiving the snapshot as if it were current records a state that stopped
being true.

Rank the sources before writing anything:

```
1  the repository now: the code, the tests, what passes today
2  facts the orchestrator carries about work done after the reports
3  verify-report and review-report
4  the original request
```

When a higher source says resolved and a lower one says open, **report the final state and
cite where it was resolved** — the commit, the later run. Do not repeat the stale claim.

When two sources contradict and neither outranks the other, **record the contradiction**:
both statements, where each came from, and when each was written. Never resolve it
silently in either direction. A contradiction written down is a question someone can
answer; one resolved by guessing is an error nobody can find.

Attribute what comes from a snapshot, rather than restating it as a present fact.

```
per verify-report, at verification time: 47 tests passing
```

Carry final numbers — test counts, warnings, uncovered lines — from the highest source that
covers them. Numbers copied from `verify-report` after later work changed them are wrong in
a document that outlives the change.

## Distinct failures stay distinct

Never merge two defects into one causal story because they appeared together. A cause is
recorded as confirmed only with evidence; otherwise the failure is recorded as
undiagnosed.

An invented causal link is worse than an admitted gap: `diagnose` will recall it as a
precedent, and the next investigation starts from a conclusion nobody proved.

## Features: merging the delta

The delta in `docs/changes/{change}/spec.md` is folded into `paths.master_specs`.

```
ADDED      the requirement is appended to its specification
MODIFIED   the existing requirement is replaced, and the Previously line is dropped
REMOVED    the requirement is deleted
```

The master specification describes current behaviour, in the present tense, with no history
of how it got there. Git holds the history, and a specification carrying its own changelog
becomes unreadable by the fifth change.

The change directory stays, and the configuration decides what is in it.

```
keep         the proposal, design, tasks and reports, plus the record
ephemeral    the record and the delta spec
pointer      the address file, and the record
```

Either way it is what a reader follows back from a master specification to understand why a
requirement says what it says. Under `ephemeral` that path is shorter and it is the record
that answers, which is the point of writing one.

## The record

Every change writes one, in every mode. It is the document the change leaves behind, and
under `artifacts.retain: final_only` it is the only new one.

Written from `templates/docs/record.md` to the locator the orchestrator passed, which
resolves under `paths.change_records`.

```
docs/changes/login-google/record.md
```

It is written for someone arriving from a master specification months later with no other
context: what the system does now, why, how it was decided, what was required, and how it
ended. Not a list of phases that ran, and not a list of keys.

**`How it was decided` is the section that pays.** It carries the approach that was taken
and the alternatives that were rejected, with the reason each was rejected, from the design.
Under `final_only` the design document is about to be deleted, and this section is the only
part of it that survives. Without it the next change reopens a decision that was already
settled, which is the cost the whole mode exists to avoid paying.

`Result` reports the state at close, not the state when `verify` and `review` wrote their
reports. Work continues after a report is written — a finding gets fixed in a later commit,
a blocked task gets finished — and a record that repeats a stale report sends the next reader
to redo finished work, or to trust that something is pending when it closed. Where the two
disagree and neither can be ranked, both are recorded with their source and time, never
resolved silently.

A change closed incomplete says so here. `archive` may close work that did not finish when
the user asks for it explicitly; it may never describe that work as finished.

## Removing the working documents

Under `artifacts.retain: final_only`, the working documents are removed once the record
exists.

```
removed     proposal, research, design, tasks, diagnosis,
            verify-report, review-report, inputs/
kept        record.md
            spec.md, under artifacts.keep_delta_spec
            paths.master_specs, architecture.md, code_conventions.md
```

They are removed from the working tree **before staging**, so they never enter the commit
and the closing diff shows the record, the delta, the merged specifications and the code.

A document an earlier pass committed is tracked, and its removal is staged the same way the
state file's is. A change that ran straight through never committed them, and they are
simply deleted.

This is the part to be honest about: with a memory backend configured, the documents were
indexed as each phase completed and memory is the copy. With `backend: none` there is no
copy, and they are gone. That is what the mode means, and it is the right trade only because
what a later reader needs was lifted into the record first. `init` says so when the mode is
chosen against no backend, and `alfred doctor` repeats it.

Order matters and is not negotiable:

```
1  merge the delta into paths.master_specs
2  write the record
3  read the record back and check it is not empty and carries How it was decided
4  only then remove the working documents
```

Step 3 is the whole safety of the operation, and it is the same check `reindex` makes before
deleting under `pointer`. A record that was written empty, or written without the decisions,
is indistinguishable from a correct one right up to the moment the inputs are gone. A failed
read-back stops the phase, deletes nothing, and names the record.

`retain: all` skips this section entirely and is the behaviour of every earlier version.

## Removing the state file

State exists so `continue` can resume. A change that closed has nothing to resume.

Under `artifacts.state_on_completion: delete`, this phase removes
`.alfred/state/{change}.yaml`.

Whether that removal is staged depends on whether the file was ever committed. A change
that ran and closed in one pass never committed its state, so the file is untracked and
deleting it is the whole operation. A change that was interrupted and resumed — the case
the file exists for — was committed by an earlier pass, and the removal has to be staged or
the file returns on the next checkout and `continue` finds a change that finished months
ago.

```
tracked     git rm, staged with the closing commit
untracked   delete it
```

The phase checks which it is rather than assuming: `git rm` on an untracked file fails the
commit, and a plain delete on a tracked one leaves the repository asserting the change is
still open.

It is removed only when the change actually closed:

```
closed complete      removed
closed incomplete    kept, and the record says what was left open
verify failed        this phase never ran, so it is kept
review blocking      same
abandoned            nothing closed it, so it is kept
```

The second row is the one that looks inconsistent and is not. Open work is work somebody may
come back to, and a record reading "three tasks unfinished" beside no state leaves nothing
able to resume them. See `skills/_shared/state-contract.md`.

## Closing the address file

Under `pointer`, every phase before this one appended a row to
`docs/changes/{change}/README.md` as it completed. This phase closes it, from
`templates/docs/addresses.md`: the remaining rows, and the postmortem key for a bug.

It is not the description of the change — the record is, and it sits beside this file in
every mode. The address file is the index that makes the entries reachable, and it stops
where the record starts. The outcome, the commit and the specifications that changed are
written once, in the record, because two documents carrying the same outcome go out of step
at the first correction with nothing to say which is current.

Both are staged together, for the reason the state file is: an index whose record is missing
describes documents nobody can place, and a record with no index under `pointer` names
nothing that can be fetched.

## Bugs: the postmortem

A bug writes a postmortem to memory rather than to the specifications.

```
remember(
  key:   alfred/postmortem/{slug}
  type:  postmortem
  title: what failed, in the terms someone would search for
)
```

```markdown
**Symptom**: what was observed
**Root cause**: what actually caused it
**Fix**: what changed
**Why it was not caught**: the gap that let it through
```

`Why it was not caught` is the field that pays. The others explain one bug; this one names
the missing test, the unwritten scenario or the wrong assumption that will let the next one
through.

`diagnose` recalls these before investigating, so a class of failure is diagnosed once.

A bug whose fix changed observable behaviour also carries a delta, and that delta is merged
as above.

## Committing

Per `git` in the configuration:

```
stage the files reported by apply
stage the record, the delta spec and the merged master specifications
stage the removal of the state file, when the change closed complete
one commit for the whole change
conventional commit message
no AI attribution of any kind
push
```

One commit, still. The working documents were already removed from the working tree, before
staging, so there is nothing to un-stage and no second commit to tidy up after the first.

A removal is staged explicitly. `git.stage: files_changed_by_alfred` names files Alfred
wrote, and a file Alfred deleted is not one of them: left unstaged, the state file is
committed as though it still existed and comes back on the next checkout.

Staging everything present would sweep in whatever the user left in progress. The file list
comes from the subagent reports, which is why `apply` requires it.

State the scope as what is excluded, never as what is included. "Stage the source and the
tests" is read literally by something that has no way to know a locale file, a migration or
a configuration key was part of the work — observed twice in one run, both times leaving the
change incomplete in the commit.

```
everything the change needs, excluding .alfred/ and docs/changes/
```

That exclusion scopes the work `apply` produced, which is code. Alfred's own documents are
not swept in by it: the record, the delta spec and the merged specifications are staged by
name, on the line above, and the state file's removal with them. Naming them is what keeps
the two rules from cancelling each other — an exclusion broad enough to keep the working
documents out of the commit is broad enough to keep the record out too.

**Compare before committing.** What git reports as changed is compared against the union of
every `files_changed` from `apply`, and any file in one and not the other is reported:

```
changed and not reported   apply touched a file and did not say so
reported and not changed   a task reported work it did not do
```

Neither is fatal and neither is resolved silently. A file changed and not reported is the
collision `apply` is built to detect, and the same difference is what `worktree.sh close`
refuses on afterwards — by then the commit already exists and the fix is another commit.
Compared here, it is a question asked while the answer is still cheap.

Alfred's own documents are outside that comparison. `apply` never wrote them, so every one
of them is "changed and not reported" by construction, and including them would report the
record as a collision on every change.

State is staged with the change — its removal when the change closed complete, its current
contents otherwise, so a collaborator who clones mid-change can continue. Under `pointer`
the address file is staged with it: they answer the same question from two
sides and neither is useful alone, per `skills/_shared/phase-protocol.md`. The skill
registry is not: it is ignored, per `skills/_shared/skill-resolver.md`.

```
feat(auth): sign in with Google

Implements docs/changes/login-google/record.md.
3 requirements, 7 scenarios, 47 tests.
```

## The record does not carry its own commit hash

It cannot. A commit's hash covers the tree it contains, so a file inside that tree can never
state the hash — writing it in changes it, and amending to correct it changes it again.

Git already answers the question from both sides, and neither needs a hash in the document:

```
which commit closed this change      git log --oneline -1 -- <the record>
which record does this commit close  the commit message names it
```

The record's `commit:` field therefore carries the commit's subject line, which is decided
before the hash exists and is stable under amend. A phase that finds itself amending to
write a hash into the tree that produced it has hit the regress, not a step it missed.

The message names the record rather than the directory. Under `final_only` the directory
holds the record and the delta and nothing else, and a message pointing at a directory that
used to hold eight documents reads as though seven went missing.

## Changes in their own worktree

When state carries a `worktree`, this phase runs inside it, and the commit lands on
`branch`.

**The delta is merged inside the worktree**, into the copy of `paths.master_specs` that is
on this branch, and it goes into this change's commit like every other file. Not into the
main checkout.

Writing to the main checkout from a worktree is wrong three times over. The merged
requirement is not in the commit, so the branch describes behaviour its own specification
does not mention. Two changes archiving close together write the same file with no branch
between them and one silently wins. And the main checkout ends up dirty, which is the state
the worktree layout exists to prevent, discovered later by whoever works there next.

Merged on the branch, two changes that touch the same requirement produce a merge conflict
when the branches meet, which is a question git puts to a person rather than a loss nobody
sees.

Under `artifacts.committed: false` there is no commit to put them in. The delta is still
merged in the worktree, and then this phase copies `paths.master_specs`, `paths.changes`
and `paths.change_records` back to the main checkout before any close — they are untracked,
so the worktree is the only place holding them and `close` discards untracked files.
`worktree.sh close` refuses while the main checkout is missing any of them, and names what
it found. See `docs/artifacts.md`.

`paths.change_records` is in that list even though it usually resolves inside
`paths.changes`, because it does not have to. Pointed somewhere else, the record is the one
document of the change and it would be the one thing `close` discarded.

This is also where `retain: final_only` and `committed: false` have to be done in the right
order. The working documents are removed first, in the worktree, and only what remains is
copied back. Copying first and deleting afterwards would carry eight documents into the main
checkout to delete seven of them there, and a `close` racing that deletion refuses on files
the main checkout has and the worktree no longer does.

After the push:

```
git.worktrees.pull_request: true    open a pull request from branch to base, if the
                                    tool for it is available; report and continue if not
git.worktrees.remove_on_archive     run worktree.sh close --branch <branch> --cwd <main_checkout>
```

`close` refuses a worktree with uncommitted changes, which after a correct commit means a
file `apply` did not report. Stop and say which files, rather than forcing: a file changed
and not committed is the collision `apply` is designed to detect. The branch is deleted
only when already merged into `base`; otherwise it stays for the pull request. Run `close`
from `main_checkout`, never from inside the worktree being removed.

The pull request is off by default because opening one is a decision about the team's
review flow, and because it needs a tool that is not always logged in. A failed attempt is
reported in the completion line, never treated as a failed archive.

## Memory conflicts

Every conflict a phase recorded during the change, per `memory/CONTRACT.md`, is collected
here into one list: what disagreed, which key, and which phase raised it.

**Settle the ones that can be settled.** Every phase is given the backend's full tool list,
so where the backend offers a judgment call this phase makes it, per
`memory/adapters/engram.md`. What reaches the user is what judgment could not decide.

This is the step that used to be skipped for the wrong reason. A phase without the tool
reads exactly like a backend that cannot settle anything, so a run recorded six open
conflicts, then twenty, and reported them as if that were the contract working. It was a
tool list written before the tool existed.

What cannot be settled is reported, not guessed: what disagreed, which key, which phase
raised it. The user settles those in one pass, or leaves them.

Conflicts raised between the artifacts of the same change — a spec that "contests" its own
design — are a similarity threshold wanting tuning, not a disagreement. Say so once, with
the count, rather than listing each.

A list of conflicts never blocks an archive. What they are about is written down twice
already.

## Carrying findings forward

`should fix` and `suggestion` findings from `review` are written to memory against the area
they belong to, not discarded. They surface the next time that area is worked on, which is
the only moment they are worth acting on.

## Workspaces

At repository level this phase completes that repository's part and commits it.

The workspace change is archived only once every repository in `repos` has passed `verify`
and `review`. Until then the workspace change stays open and `status` names the repository
holding it. See `skills/_shared/workspace-protocol.md`.

## Completion

Follow `skills/_shared/phase-protocol.md`: write the merged specifications, index them,
update state to `completed`, notify `feature_completed`.

```
archived: 3 requirements merged, committed as feat(auth): sign in with Google
```
