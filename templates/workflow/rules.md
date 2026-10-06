# {title}

The judgement this workflow applies. The orchestrator reads this file before it proposes a
route, and it is the only file of this workflow it opens while a change is running: the
structure — the phases, the routes, the default, the entry points, the groups dispatched
together and the phase that closes a change — was read from `workflow.json` when the command
was registered, and the command carries it.

Nothing here restates a structural fact. A route written in both places has two authorities,
and the wrong one is the one nobody re-reads.

## Choosing a route

### {route-1}

What kind of request deserves this route, and the signals in the request that say so. Write
signals that can be observed before any work starts — what the user asked for, what exists
already, what would change — rather than conclusions a phase reaches later.

### {route-2}

The same, for the other route. Two routes whose signals overlap leave the choice to
whichever is written first; say what separates them.

## How a route changes mid-run

What a phase can discover that makes the route it is on the wrong one, and what happens
then. That a route is proposed and never applied silently, that the user may always shorten
one, and that it is never lengthened without saying so, are mechanics and hold under every
workflow. What belongs here is this workflow's own reason to change one.

## External material

Which phase receives a tracker card, a URL or a document from another system, reads it once
and writes it to the change's `inputs/` as text. Name one phase: material fetched twice is
material two phases can disagree about.

## What this workflow does not handle

The kinds of request that belong to another workflow. The orchestrator names the workflow a
request belongs to rather than running it here, and this is the list it names them from.
