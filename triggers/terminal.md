---
trigger: terminal
blocking_capable: true
---

# Terminal trigger

Starting a run from the shell, in OpenCode, Claude Code, or any agent where Alfred is
registered.

## Starting

```
alfred init                     set up this repository
alfred "add google sign-in"     start a change
alfred bug "payments time out"  start from a defect
alfred continue                 resume where it stopped
alfred status                   what is open and where it is
alfred ff spec                  skip ahead to a phase
```

`alfred` with a description is the normal entry point. Routing decides which phases run, per
`skills/_shared/routing.md`.

In an agent with an agent picker, the same is reached by selecting the Alfred orchestrator
and describing the work. There is no separate command to learn.

## Origin channel

A run started here records `origin_channel: terminal`, and blocking questions are asked
here. See `notify/CONTRACT.md`.

## Resuming

A run interrupted mid-phase leaves state at that phase. `alfred continue` reads it and
resumes, including a question that was waiting for an answer.

Nothing is lost by closing the terminal. State and documents are files.

## Change names

Derived from the description as a slug, and confirmed.

```
"add google sign-in"  ->  login-google
```

The name is the directory under `docs/changes/` and the prefix of every memory key for the
change, so it is chosen once and never changes.
