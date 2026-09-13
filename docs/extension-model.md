# Extension model

## Goal

Atoha should gain integrations and policies without turning the core into a
framework or a device model. v0.1 uses in-process Go composition and the
smallest contracts that real experiments require.

## Extension roles

### Provider

Dispatches an action to a physical execution backend and returns all available
dispatch evidence. The required interface must remain minimal. Discovery,
observation, cancellation, subscriptions, and capability reporting are not
automatically Provider methods.

For v0.1, the required contract is the single `Provider.Dispatch` method.
`Resolver`, `Admitter`, `Observer`, and `Verifier` are independent interfaces;
implementations advertise them through ordinary Go interface satisfaction.

### Resolver

Maps a logical target to a provider endpoint. Resolution is separate from
device discovery and from a universal capability taxonomy.

### Admitter

Evaluates whether an action may proceed and returns an explainable decision.
Atoha invokes the enforcement point; extensions own authorization,
confirmation, safety, and domain policy.

### Observer

Obtains timestamped evidence from a provider or independent source. Keeping
observation separate allows command and evidence paths to differ.

### Verifier

Evaluates whether observations satisfy an Action's desired effect under an
explicit policy. A provider acknowledgement alone is not verification unless a
domain-specific verifier deliberately treats it as sufficient evidence.

### EventSink

Receives execution facts for local logs, SQLite, telemetry, or cloud sync. The
contract must not require a particular broker or event-sourcing product.

## Interface design constraints

- Prefer one-method or narrowly cohesive interfaces.
- Make optional capabilities separate interfaces.
- Define zero values and ownership of byte slices/maps precisely.
- Preserve provider evidence and timestamps.
- Keep provider-specific data opaque at the core boundary where possible.
- Do not add an abstraction based only on one Home Assistant implementation.
- Supply contract tests once the behavior is stable enough to state.

A large Provider interface combining execution, discovery, observation,
verification, subscription, and cancellation is explicitly out of scope.

## Evolution

### Phase 1: in-process composition

Compile provider implementations with the server. This keeps iteration cheap
while Action, Outcome, and Observation semantics are tested.

### Phase 2: provider processes

Consider independently distributed executables only after a stable boundary is
visible. Candidate local transports include stdio, Unix sockets, local HTTP,
and gRPC. Selection requires an RFC covering lifecycle, protocol versioning,
credential handling, cancellation semantics, streaming observations, and
failure isolation.

Do not use Go's standard `plugin` package as the ecosystem boundary because its
build and platform compatibility constraints conflict with independently
released providers.

### Phase 3: ecosystem tooling

Manifests, registries, templates, conformance suites, and separate provider
repositories follow demonstrated third-party needs. They are not v0.1
prerequisites.

## First evidence source

The Home Assistant proof of concept should exercise light, climate, and
media-player entities as fixtures, not core types. It should record traces for:

- accepted command with a fresh matching observation;
- accepted command with no observation;
- transport timeout with possible dispatch;
- stale or conflicting observation; and
- an action whose replay is unsafe.

Those traces inform the contracts; the contracts must not merely mirror the
Home Assistant API.
