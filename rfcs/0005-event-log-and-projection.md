# RFC 0005: Event log and projection

- Status: Accepted
- Authors: Habio maintainers
- Created: 2026-09-06
- Related issues: #6

## Summary

Record immutable execution facts through a one-method EventSink and derive
replaceable current views by replaying a sink's canonical order. Provide only a
small in-memory reference log and attempt projection in v0.1.

## Motivation

Mutating one status value loses late, contradictory, and independently sourced
evidence. Habio needs facts for audit and reconstruction, but adopting a broker
or full event-sourcing platform before validating the facts would grow the core
around infrastructure rather than semantics.

## Proposal

ExecutionEvent records EventID, ActionID, optional AttemptID, EventKind,
OccurredAt, RecordedAt, and opaque data. ActionRequested may precede attempt
creation; other v0.1 facts identify an attempt. Event kinds state facts rather
than commands.

EventSink has one Append method. Durable sinks define their canonical replay
order and delivery guarantees. The reference memory log preserves append order,
treats an identical EventID and event as an idempotent duplicate, and rejects
reuse of an ID for different content.

The reference Attempt projection consumes canonical replay order. Weaker late
dispatch evidence does not replace stronger evidence. Incompatible admission,
dispatch, or effect claims mark the view conflicted and return that dimension
to unknown rather than inventing certainty. All original facts remain in the
log. Once conflicted, a dimension remains unknown during ordinary replay; v0.1
defines no implicit conflict resolution rule. The projection exposes both an
aggregate conflict flag and flags for each outcome dimension.

OccurredAt reflects when the source says the fact occurred. RecordedAt reflects
Habio ingestion. Neither timestamp by itself defines distributed total order.

## Alternatives

### Store only current Outcome

This discards evidence and prevents reconstruction after projection logic
changes. Rejected.

### Require Kafka or another broker

This violates local-first operation and makes infrastructure the abstraction.
Rejected.

### Define a complete event-sourcing framework

Snapshots, schemas, upcasters, subscriptions, and distributed ordering are not
needed to prove v0.1 semantics. Deferred.

### Reorder by OccurredAt

Source clocks can skew and late facts can arrive. Sinks must expose a canonical
replay order; timestamps remain evidence rather than an invented total order.

### Last write always wins

This can convert contradictory physical evidence into false certainty.
Incompatible claims become an explicit projection conflict.

## Drawbacks

The initial EventKind vocabulary may evolve. Opaque data requires extension
schema ownership. Replay order must be specified by each durable sink. A memory
log is not crash-safe.

## Open questions

- Which event payload fields should become cross-implementation conformance requirements?
- How are facts synchronized without treating cloud order as physical order?
- When can a later verification resolve a previously conflicted dimension?

## Decision

Accepted for v0.1. Keep the core contract to immutable facts and Append. Treat
the memory log and Attempt projection as replaceable reference implementations,
not required infrastructure. RFC 0007 adds a stronger application-owned Journal
contract and a local SQLite implementation without expanding the core contract.
