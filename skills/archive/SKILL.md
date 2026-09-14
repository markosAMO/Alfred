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

The change directory stays. It is the record of one change, with its proposal, design,
tasks and reports, and it is what a reader follows back from a master spec to understand
why a requirement says what it says.

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
stage only the files reported by apply
one commit for the whole change
conventional commit message
no AI attribution of any kind
push
```

Staging everything present would sweep in whatever the user left in progress. The file list
comes from the subagent reports, which is why `apply` requires it.

```
feat(auth): sign in with Google

Implements docs/changes/login-google.
3 requirements, 7 scenarios, 47 tests.
```

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
