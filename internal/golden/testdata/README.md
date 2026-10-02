# Recorded output

These files are what the installer's helper emits, byte for byte. They are what it has to
keep emitting until a change means otherwise.

`fixture.json` is the input every recording was made from: the tree to build, the payload,
and the edits that produce one file of every classification a comparison can return. It
holds a name that orders differently as a path than as a string, a `.git` directory, the
two files an installation writes itself, and a non-ASCII name.

`profiles/` holds the three profile shapes: the uniform one the installer writes when memory is declined, one that
sets every optional key with the phases out of alphabetical order, and one that omits the
optional keys and switches the memory prefix off.

## Re-recording

Only when the output is meant to change. A diff here is a change to a document another
program reads, so it belongs in the commit that justifies it.

```
go build -o .build/alfred ./cmd/alfred
go test ./internal/golden/ -v          # read the diff first, then decide
```

There is no `-update` flag on purpose: overwriting a recording has to be a decision, not a
flag someone reaches for when a test goes red.
