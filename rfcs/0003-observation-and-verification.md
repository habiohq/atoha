# RFC 0003: Observation and Verification

- Status: Accepted
- Authors: Habio maintainers
- Created: 2026-09-06
- Related issues: #4

## Summary

Treat Observation as timestamped, source-attributed evidence and Verification
as a named policy's immutable assessment of one Action against observations at
an explicit evaluation time.

## Motivation

Provider state can be stale, indirect, delayed, or inconsistent with an
independent sensor. A command acknowledgement is not proof of the desired
effect. Habio needs to retain evidence and identify the policy that judged it.

## Proposal

Observation contains opaque identity, source, logical target, value, optional
source evidence, ObservedAt, and RecordedAt. Values and evidence are bytes owned
by the Observation. ObservedAt represents the source clock; RecordedAt
represents Habio ingestion. Clock skew is preserved rather than silently
corrected.

Freshness is not a boolean stored on Observation because freshness depends on
the action, verifier, and evaluation time. Verifier receives an explicit `asOf`
time and decides whether evidence is fresh enough.

VerificationResult is unknown, verified, unsatisfied, or inconclusive. A
non-unknown result records verifier identity, check time, observation IDs, and
a diagnostic reason. Inconclusive covers missing, stale, contradictory, or
otherwise insufficient evidence and may therefore have no observation IDs.
Verified and unsatisfied results require at least one supporting observation
ID. Reason is not a control-flow API.

Verifier is a one-method extension contract. Concrete comparison, tolerance,
safety, and freshness policy stays outside core.

## Alternatives

### Treat provider state as truth

This cannot express stale caches, indirect evidence, or conflicting sensors.
Rejected.

### Store `Fresh bool`

Freshness changes with evaluation time and policy, so a stored boolean becomes
stale itself. Rejected.

### One timestamp

Using only ingestion time overstates freshness when a provider returns an old
state. Using only source time hides delivery delay. Both are retained.

### Boolean verification

True/false cannot distinguish contradictory evidence from missing or stale
evidence. Rejected.

### Put Verify on Provider

Observation and verification can use independent sources and policies. The
contracts remain separate.

## Drawbacks

Opaque values require extension-specific decoding. Source clocks can be wrong,
so Verifiers must explicitly handle future observations and skew. Observation
IDs add persistence and correlation work for runtimes.

## Open questions

- Do conformance formats need media types or schemas for Value and Evidence?
- How should a projection expose multiple simultaneous verifier results?
- When should contradictory observations create an explicit conflict fact?
- Does verifier identity require a separate version field?

## Decision

Accepted for v0.1. Evidence remains immutable and source-attributed. Freshness
and desired-effect semantics remain explicit Verifier policy.
