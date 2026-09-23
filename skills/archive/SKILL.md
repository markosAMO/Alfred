---
name: archive
mode: auto
skippable: false
reads: [spec, diagnosis, verify_report, review_report, master_specs]
writes: [master_specs, postmortem, commit]
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

The change directory stays, and `memory.documents` decides what is in it. Under `keep` it
holds the proposal, design, tasks and reports. Under `pointer` it holds the address file,
and the documents it names are in memory. Either way it is what a reader follows back from
a master spec to understand why a requirement says what it says.

## Closing the address file

Under `pointer`, every phase before this one appended a row to
`docs/changes/{change}/README.md` as it completed. This phase closes it, from
`templates/docs/addresses.md`: the one-line description, the outcome, the commit, the
specifications that changed, and the postmortem key for a bug.

It is the only description of this change that stays in the repository, so it is written
for someone arriving from a master specification with no other context, not as a list of
keys.

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
one commit for the whole change
conventional commit message
no AI attribution of any kind
push
```

Staging everything present would sweep in whatever the user left in progress. The file list
comes from the subagent reports, which is why `apply` requires it.

State the scope as what is excluded, never as what is included. "Stage the source and the
tests" is read literally by something that has no way to know a locale file, a migration or
a configuration key was part of the work — observed twice in one run, both times leaving the
change incomplete in the commit.

```
everything the change needs, excluding .alfred/ and docs/changes/
```

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

State is staged with the change, so a collaborator who clones mid-change can continue, and
under `pointer` the address file is staged with it: they answer the same question from two
sides and neither is useful alone, per `skills/_shared/phase-protocol.md`. The skill
registry is not: it is ignored, per `skills/_shared/skill-resolver.md`.

```
feat(auth): sign in with Google

Implements docs/changes/login-google.
3 requirements, 7 scenarios, 47 tests.
```

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
merged in the worktree, and then this phase copies `paths.master_specs` and
`paths.changes` back to the main checkout before any close — they are untracked, so the
worktree is the only place holding them and `close` discards untracked files.
`worktree.sh close` refuses while the main checkout is missing any of them, and names what
it found. See `docs/artifacts.md`.

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
