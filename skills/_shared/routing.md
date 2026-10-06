# Routing

Before any phase runs, the orchestrator chooses a route, states why, and waits for the
user to accept it. This file is the part of that which is true under every workflow.

## Where a route comes from

The routes of a run, the default among them, and where each kind of request enters are the
running workflow's, declared in its definition and carried by the command the run started
from, per `skills/_shared/workflow-protocol.md`. The judgement that chooses between them —
the question to ask of a request, the signals each route matches, when a route changes
mid-run — is that workflow's too, and is written in its rules file.

Nothing here decides a route. Two workflows answer the same request with different routes
and both are right, because a route is a statement about one recipe and not about Alfred.

```
the routes there are         the workflow's definition, through the command
which one this request gets  the workflow's rules file
how it is proposed           this file
```

## A route is proposed, never applied silently

The orchestrator reads the running workflow's rules, proposes one of that workflow's
routes, and waits. A route that cannot be chosen because the rules file cannot be read
stops the run and names the file; none is applied in its place.

```
This looks like the longer of this workflow's two routes: the account has no
contact on record, and the last three messages went unanswered.

Start there, or go straight to the shorter one?
```

The signals in that example are some workflow's, not Alfred's; what this file fixes is the
shape of the proposal around them. Signals are checkable and they are not infallible, and a
route wrongly proposed costs the user work they did not need. The proposal is what makes
that correctable before it is paid for.

## Stating the route

Every route decision names the signals it was based on. A route presented without its
reasons cannot be corrected, because the user cannot see which input was wrong.

## Shortening and lengthening

The user can always shorten a route. The orchestrator never lengthens one without saying
so.

The asymmetry is deliberate. Shortening is the user declining work they judge unnecessary,
and they are the one paying for it; lengthening is the orchestrator deciding the user
needs more than they asked for, which is a judgement they are entitled to see before it
runs.

## A route the workflow does not have

Asked for a route the running workflow does not declare, the orchestrator says it does not
exist in this workflow and lists the ones that do. It never assembles one out of the
phases: a route nobody declared has no rules file passage behind it and no group, entry
point or closing phase checked against it.
