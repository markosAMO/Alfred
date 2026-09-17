---
trigger: terminal
blocking_capable: true
---

# Terminal trigger

Starting a run from the shell, in OpenCode, Claude Code, or any agent where Alfred is
registered.

## Starting

Reach the orchestrator in your agent and describe the work. These are asked of the
orchestrator, not typed into a shell: there is no global `alfred` command.

| Agent | Orchestrator |
|---|---|
| Claude Code | the `/alfred` command |
| OpenCode | the `alfred` primary agent, on Tab |

```
init                        set up this repository
add google sign-in          start a change
bug: payments time out      start from a defect
continue                    resume where it stopped
status                      what is open and where it is, across worktrees
ff spec                     skip ahead to a phase
abandon feature/login       drop a change running in its own worktree
```

Several changes at once are started with `/alfred-worktree` in Claude Code, one change per
line, each on a named branch: `feature/login-google from main: add google sign-in`. Each
runs in its own worktree. See `skills/_shared/worktree-protocol.md`.

A plain description is the normal entry point. Routing decides which phases run, per
`skills/_shared/routing.md`.

Installing and updating Alfred itself is separate, and is `./install.sh` from the clone.
See `docs/installation.md`.

## Origin channel

A run started here records `origin_channel: terminal`, and blocking questions are asked
here. See `notify/CONTRACT.md`.

## Resuming

A run interrupted mid-phase leaves state at that phase. asking the orchestrator to `continue` reads it and
resumes, including a question that was waiting for an answer.

Nothing is lost by closing the terminal. State and documents are files.

## Change names

Derived from the description as a slug, and confirmed.

```
"add google sign-in"  ->  login-google
```

The name is the directory under `docs/changes/` and the prefix of every memory key for the
change, so it is chosen once and never changes.
