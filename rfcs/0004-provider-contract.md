# RFC 0004: Provider extension contract

- Status: Accepted
- Authors: Habio maintainers
- Created: 2026-09-06
- Related issues: #5

## Summary

Make Provider a one-method dispatch contract. Keep target resolution,
admission, observation, and verification in separate interfaces. Dispatch
returns evidence and a Go error independently.

## Motivation

Provider implementations absorb hardware complexity, but a large integration
interface would couple core to discovery, state subscriptions, cancellation,
and one platform's capabilities. The execution boundary needs only a way to
dispatch one attempt and retain what is known when the call returns.

## Proposal

`Provider.Dispatch` accepts context, immutable ExecutionAttempt and Action, and
an opaque ResolvedTarget. It returns DispatchResult and error. Context controls
the software wait; cancellation does not assert physical cancellation.
Before external I/O, a Provider must reject an ExecutionAttempt whose ActionID
does not match the Action. A provider-specific implementation must also verify
that ResolvedTarget is valid for that Action rather than trusting a caller to
keep independently supplied values paired correctly.

DispatchResult identifies provider and attempt and carries DispatchStatus. An
acknowledged result requires a matching Receipt. Receipt contains provider,
attempt identity, receive time, and opaque acknowledgement evidence. A Provider
can return both evidence and an error.

Resolver maps Action to an opaque provider endpoint. Admitter returns admission
knowledge. Observer obtains Observations independently. Verifier remains the
contract accepted in RFC 0003. Optional capabilities are discovered through Go
interface assertions during v0.1; no capability manifest is required.

The zero DispatchResult means no provider evidence was returned. A constructed
result always identifies its attempt and provider.

## Alternatives

### Large Provider interface

Combining execute, observe, verify, discover, subscribe, cancel, and capability
methods forces unrelated implementations to stub behavior and makes the core
mirror a hardware platform. Rejected.

### Provider resolves logical targets internally

Some providers will do this, but an explicit Resolver permits portable binding
and independent providers. A provider may still resolve to an empty opaque
endpoint when its own configuration uses the logical target.

### Error-only dispatch

An error cannot preserve acknowledgement received before a later decode or
transport error. Rejected.

### Standard Go plugins

Build, OS, and Go-version coupling conflicts with independently distributed
extensions. Rejected as the ecosystem boundary.

### Process protocol in v0.1

Stdio, Unix socket, HTTP, and gRPC choices require lifecycle and versioning
evidence that one provider cannot supply. Deferred.

## Drawbacks

Adapters must compose several small interfaces. Opaque endpoint and receipt
bytes require provider-specific encoding. Go interface assertions provide no
cross-process capability discovery.

## Open questions

- Which process protocol best preserves partial results and late evidence?
- Does Receipt need an external reference or media type after two providers?
- How should streaming observations and provider lifecycle be exposed?
- What conformance behavior can be tested without prescribing transport?

## Decision

Accepted for v0.1. Add capabilities as separate contracts only after an
implementation demonstrates the need. Process isolation and manifests require
a later RFC.
