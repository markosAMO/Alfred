---
trigger: openclaw
blocking_capable: true
---

# OpenClaw trigger

Starting a run from WhatsApp or Telegram, through an OpenClaw assistant.

The assistant does not run the pipeline. It recognises the intent, picks the repository, and
starts the orchestrator there. Everything after that is the same pipeline the terminal runs.

## What the assistant does

```
1  recognise that the message is a software request
2  resolve which repository it refers to
3  start the orchestrator in that repository with the request
4  relay what comes back
```

Step 2 is the one that needs configuring. A message says "the payments thing", not a path.

```yaml
repositories:
  billing: ~/Work/api-billing
  front: ~/Work/front
  work: ~/Work
aliases:
  payments: billing
  checkout: front
default: work
```

`work` being a directory of repositories means an unqualified request starts at workspace
level, where `design` decides which repositories are involved. See
`skills/_shared/workspace-protocol.md`.

An ambiguous reference is asked about rather than guessed. Starting a change in the wrong
repository is cheap to undo and expensive to notice.

## Message shapes

```
"alfred, armemos el spec del login con google"      -> refine, in the default repository
"alfred, en billing: los pagos tiran timeout"       -> diagnose, in billing
"alfred, implementá"                                -> continue the open change
"alfred, cómo viene?"                               -> status
```

The assistant translates intent into a command. It does not decide the route: that is the
orchestrator's, and it is proposed back to the user.

## Origin channel

A run started here records `origin_channel: openclaw`. Blocking questions come back to the
same conversation, per `notify/CONTRACT.md`.

Without that, a run started from a phone blocks on a question printed to a terminal nobody
is watching, and waits forever.

## Long waits

A phase in `interactive` mode may wait hours for a reply from a phone. The run stays in
`waiting_for_input` with the question recorded in state. Answering later resumes it; the
session ending in between changes nothing.

## What it never does

The assistant does not write code, specifications or tasks, and does not summarise a phase's
output in its own words. It relays what the pipeline produced.

A summary of a summary is how a user ends up approving something that was never said.
