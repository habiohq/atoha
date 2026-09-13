# Atoha design

## Purpose

Atoha owns the action boundary: the point at which an intent crosses into a
physical execution system and produces incomplete, delayed, or contradictory
evidence about the result. Its job is to report that evidence without inventing
certainty.

The core should remain useful for a light, an EV charger, an HVAC system, or a
robot without containing a model of any of them.

## Semantic model

### Action

An Action is an immutable expression of intent. It needs an identity, a logical
target, an operation name, and typed or encoded input. Natural language is
translated before this boundary. Provider endpoint IDs do not belong in a
portable Action unless the caller intentionally chooses a provider-specific
target.

Action identity supports correlation and deduplication; it does not prove that
repeating the Action is safe.

### ExecutionAttempt

An ExecutionAttempt records one attempt to carry out an Action. Attempt
identity must be separate from Action identity so that an explicit recovery can
be traced without rewriting history. Timestamps, provider selection, dispatch
evidence, and terminal knowledge belong to the attempt.

### Receipt

A Receipt is provider evidence about dispatch or acceptance. Its meaning is
provider-specific and must be described rather than promoted to physical
success. A provider acknowledgement may prove acceptance while proving nothing
about the resulting world state.

### Observation

An Observation is evidence obtained from a provider or physical-world source.
It can carry a source, observation time, ingestion time, value, freshness
information, and source evidence. An observation is not absolute truth: it may
be stale, indirect, or inconsistent with another source.

### Verification

Verification evaluates whether available observations satisfy the intended
effect under an explicit verifier. Verifiers are extensions because sufficient
evidence is domain-specific. For example, observing an HVAC setpoint of 24 C is
different from observing the room reach 24 C.

### Events and projections

Commands express intent. Events record facts. State is a projection.

Potential facts include ActionRequested, ActionAdmitted, ActionDispatched,
ProviderAcknowledged, ObservationRecorded, ActionVerified, and OutcomeUnknown.
This vocabulary is illustrative, not yet a stable API. Projections such as a
current status, audit view, or metric should be reconstructable from recorded
facts where practical.

This principle does not require Kafka, a distributed event store, or a complete
event-sourcing framework. The v0.1 implementation should prove which facts are
needed before choosing storage machinery.

## Outcome is not a boolean

Dispatch knowledge, physical knowledge, and verification are related but not
interchangeable. A single `success` value loses information. The design must be
able to express at least:

- rejection before dispatch;
- known dispatch or provider acknowledgement;
- observed desired or undesired state;
- verified desired effect;
- known failure with evidence; and
- unknown or ambiguous outcome.

The exact Go representation remains an open design question. An orthogonal
model (dispatch result, observed physical result, verification result) may avoid
the false ordering implied by one large enum. The Outcome RFC must test both
approaches against real Home Assistant traces before stabilizing the API.

A Go `error` reports that an operation in the software path did not complete as
expected. It does not, by itself, prove physical failure. APIs must return
physical knowledge separately from transport or implementation errors.

## Ambiguity and recovery

If a request times out after dispatch, Atoha may be unable to distinguish among
non-arrival, acceptance without execution, execution, and execution with a lost
response. The honest result is unknown. Atoha must not automatically map this
case to failed and retry it.

Recovery belongs to an explicit caller decision informed by evidence and replay
safety. Replay safety can differ at four levels:

- protocol;
- provider;
- device; and
- resulting world state.

`set_temperature(24)` may be relatively replay-safe; `toggle`, `dispense`, or
`move_forward(1m)` may not be. Action identity must not hide those differences.

See [docs/failure-model.md](docs/failure-model.md).

## Extension contracts

The core supplies narrow contracts and enforcement points. Integrations and
policy live outside it:

- Provider: dispatches to a physical execution backend.
- Resolver: maps a logical target to a provider endpoint.
- Admitter: allows, denies, or requires an external decision before execution.
- Observer: obtains evidence from a provider or another source.
- Verifier: evaluates Action plus Observation evidence.
- EventSink: receives execution facts.

The first implementation uses small in-process Go interfaces. Process plugins,
manifests, dynamic discovery, and capability negotiation are deferred until
multiple implementations demonstrate a stable shared contract. Go's standard
`plugin` package is not a portability target.

See [docs/extension-model.md](docs/extension-model.md).

## Enforcement without policy ownership

Atoha owns the point at which an action may be admitted, but not the policy
itself. Authorization, confirmation, safety bounds, energy constraints, and
domain rules are supplied by the application or extensions.

> Own the enforcement point, not the policy.

## Strategy boundary

Strategies consume data and produce typed Actions. The execution core does not
know their business logic:

```text
data sources -> strategy -> typed Action -> Atoha -> provider -> physical world
```

Natural-language strategy authoring, simulation, portability, binding,
conflicts, and marketplaces are valuable future layers. They must not enter the
v0.1 execution core. AI may draft policy; a deterministic runtime should execute
approved policy. The accepted portability boundary is documented in
[docs/strategy-contract.md](docs/strategy-contract.md); it deliberately does
not select a runtime format.

## Explicit non-goals

The core is not a Home Assistant wrapper, MCP server, device model, driver SDK,
automation engine, digital twin, state database, LLM framework, policy catalog,
dashboard, or SaaS control plane. It does not implement Matter, MHS, MQTT,
Zigbee, Modbus, OCPP, or vendor APIs.

## Decision discipline

Semantic changes with a broad or durable effect use the lightweight RFC process
in [rfcs/README.md](rfcs/README.md). In particular, Action, Outcome,
Verification, Provider APIs, event persistence, process-provider protocols, and
strategy portability require evidence and explicit open questions.
