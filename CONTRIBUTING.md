# Contributing

Bug reports, recovery stories and patches are welcome.

## Ground rules

fbrescue is a recovery tool, and its users are often having a bad day already. Two rules follow from that:

1. **Never modify the source database.** Every command must be read-only; CI checks that no command changes the database file. If a feature cannot be done read-only, it does not go in.
2. **Never drop data silently.** Anything that cannot be read or decoded is reported (the summary, `_errors.jsonl`, exit status 2), never skipped without a word.

A third rule keeps the install story simple: no runtime dependencies — one static binary, no cgo.

## Build and test

```sh
go build ./cmd/fbrescue
go test ./...
```

The unit tests run without any setup. The full test suite needs the corpus — databases created by real Firebird 2.5, 3.0, 4.0 and 5.0:

```sh
# Ubuntu: sudo apt-get install libtommath1 libncurses6
tools/mkcorpus/mkcorpus.sh corpus
FBRESCUE_CORPUS=$PWD/corpus go test ./...
```

CI runs both suites, plus `gofmt`, `go vet`, `-race` and golangci-lint.

## Correctness over confidence

Every assumption about the on-disk format must be verified against a database created by a real Firebird engine, and listed in [docs/format.md](docs/format.md) with how it was verified. If you change what fbrescue reads or decodes, extend the corpus (see `tools/mkcorpus/`) rather than crafting bytes by hand — hand-made files tend to encode the same misunderstanding as the code.

The main export test is a round trip: the SQL export is loaded into a new database by the same Firebird version, and SELECT output must match the original exactly.

## Pull requests

- One change per PR; the commit message explains *why*, not only *what*.
- Code, comments and docs are in English.
- If a change affects the recovery rate, rerun the damage tests — the README table must stay honest.

## Good places to start

Issues labeled **good first issue** are self-contained and need no knowledge of the page format. Bigger ideas are best discussed in an issue first.

## Reporting a recovery problem

Use the "Recovery problem" issue template and attach the output of `fbrescue report` — it contains no data and anonymizes table and column names. **Never attach a database file or exported data.**
