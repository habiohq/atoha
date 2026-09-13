# Contributing to Atoha

Atoha is early. Contributions that sharpen the execution boundary and test its
semantics against real physical-system behavior are especially valuable.

## Before proposing a change

1. Read [DESIGN.md](DESIGN.md) and [ARCHITECTURE.md](ARCHITECTURE.md).
2. Search existing issues and RFCs.
3. Use an issue for a bounded change and an RFC for a durable semantic or
   ecosystem decision.
4. Keep integrations, protocols, device taxonomies, and domain policy out of
   the core.

## Development

Atoha uses Go. Until code beyond the package skeleton exists, the baseline
checks are:

```sh
gofmt -w .
go test ./...
go vet ./...
```

Do not add a dependency without explaining which execution-semantic concern it
solves. Prefer small interfaces discovered from at least two implementations.

## Pull requests

Describe:

- what the change establishes;
- what it intentionally does not decide;
- open design questions; and
- the next issues it enables.

A PR that changes public semantics should include cases for timeout after
dispatch, stale observation, disagreement between command and observation
paths, and unsafe replay where applicable. Never collapse unknown into success
or failure merely to simplify an API.

By contributing, you agree that your contributions are licensed under the
Apache License 2.0.
