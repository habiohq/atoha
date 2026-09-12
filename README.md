# Habio

Habio is open execution infrastructure for AI-controlled physical systems.

> Models decide what should happen. Hardware platforms know how to make it
> happen. Habio defines what it means for that action to have happened.

Habio is intended to be a small, deterministic execution boundary between
probabilistic software and the physical world. The first vertical is the smart
home; the execution semantics are deliberately not tied to a smart-home device
model.

## Why Habio?

In a physical system, a successful tool call does not prove a successful
physical outcome:

```text
tool call succeeded
  != command was dispatched
  != provider accepted the command
  != physical state changed
  != desired outcome was verified
```

A timeout can mean that nothing happened, or that the action happened and only
the response was lost. Blindly converting that uncertainty into failure and
retrying can be unsafe. Habio preserves the distinction and gives the caller
enough evidence to make an explicit recovery decision.

## Scope

The core owns the semantics and contracts for:

- actions and action identity;
- execution attempts and dispatch receipts;
- outcomes, including ambiguous and unknown outcomes;
- observations and verification;
- execution lifecycle facts; and
- replay-safety information.

The core does **not** own device discovery, drivers, protocols, a canonical
device taxonomy, planning, workflow orchestration, domain policy, or cloud
operations. Providers such as Home Assistant and MHS absorb hardware
complexity. MCP is an adapter above the core, not a core dependency.

## Architecture

```text
Claude / ChatGPT / custom agent
              |
          MCP / API
              |
        [adapter layer]
              |
              v
        +-----------+
        |   Habio   |
        |           |
        | Action    |
        | Attempt   |
        | Outcome   |
        | Observe   |
        | Verify    |
        +-----+-----+
              |
          Provider
      +-------+--------+
      |       |        |
     HA      MHS     custom
      |
  physical devices
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for component and repository boundaries,
[DESIGN.md](DESIGN.md) for the semantic model, [docs/runtime-api.md](docs/runtime-api.md)
for local server usage, and [docs/v0.1-scope.md](docs/v0.1-scope.md) for the
executable acceptance matrix.

## Design principles

1. Own the action boundary, not the device model.
2. Protocols and hardware integrations live at the edges.
3. The core defines execution semantics; extensions define integrations and
   policy.
4. Never turn ambiguity into certainty.
5. Local execution must not depend on the cloud.
6. If a concept can grow independently, it belongs outside the core.
7. Commands express intent. Events record facts. State is a projection.
8. Extensibility comes through contracts, not frameworks.
9. Providers absorb hardware complexity.
10. Core APIs remain small even as the ecosystem grows.

The ecosystem succeeds when it can grow without growing the core.

## Status

Habio is in its foundation phase. The repository includes provider-independent
core contracts, application use cases, memory and SQLite Journals, a strict HTTP
adapter, a local server composition root, and a Home Assistant proof of concept.
The public API is not yet stable. The proof exercises light, climate, and
media-player actions without adding those device types to core; see
[docs/home-assistant-poc.md](docs/home-assistant-poc.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
