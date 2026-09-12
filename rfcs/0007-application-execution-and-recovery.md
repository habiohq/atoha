# RFC 0007: Application execution and explicit recovery

- Status: Accepted
- Authors: Habio maintainers
- Created: 2026-09-12
- Related issues: #3, #4, #5, #6

## Summary

Coordinate core contracts in an application package with separate Execute,
Verify, Recover, Get, and incomplete-attempt use cases. Persist an immutable
Action and atomically claim an Attempt before any provider I/O. Never retry an
ambiguous dispatch automatically; recovery requires a new Attempt ID and an
explicit, recorded authorization.

## Motivation

The core types correctly distinguish admission, dispatch, effect, and
verification, but callers still need a safe order in which to use them. A
generic transaction or HTTP handler cannot safely improvise this order: a
database transaction cannot include a physical action, and a timeout after
dispatch cannot establish that nothing happened.

The coordination belongs above core. Keeping it in `execution` preserves the
core boundary while giving adapters one tested application API.

## Proposal

### Use cases

- `ExecuteAction` records ActionRequested and AttemptStarted before admission,
  resolution, or provider dispatch. It records each later fact and returns
  semantic knowledge separately from operation errors.
- `VerifyAttempt` resolves and observes independently, records observations,
  and records a named Verifier assessment. Execute does not verify immediately
  because physical state may converge later.
- `RecoverAttempt` requires prior-attempt facts plus immutable authorization
  containing Action ID, prior Attempt ID, actor, time, and reason.
- `GetAttempt` rebuilds a view from facts.
- `ScanIncompleteAttempts` reports claimed attempts without a conclusive
  dispatch fact. It never dispatches or recovers them.

### Journal contract

The application defines the Journal interface it consumes. `BeginAttempt`
atomically:

1. registers or verifies the immutable Action contents;
2. claims a unique Attempt ID; and
3. appends the initial execution facts.

If any part fails, no provider I/O occurs. Reusing an Action ID with a different
target, name, input, or requested time is a conflict. Reusing an Attempt ID
returns an already-started result and does not re-enter admission or dispatch.

`AppendAll` atomically and idempotently appends later facts by Event ID. The
reference memory and SQLite Journals implement this stronger application
contract while retaining the one-method core EventSink contract.

### Physical transaction boundary

The safe sequence is:

```text
commit Action + AttemptStarted
        -> admit
        -> resolve
        -> perform provider I/O exactly once
        -> commit returned evidence
```

No database lock or transaction remains open across provider I/O. If recording
after I/O fails, the returned `ExecuteResult` still contains all provider
evidence known in memory and the operation error reports the recording failure.
The caller must inspect both. The application does not retry.

Post-I/O recording uses a short context detached from caller cancellation. This
allows known evidence to be retained after a client disconnect without allowing
unbounded background work.

### Event data

Application-emitted opaque event data uses JSON schema identifier
`habio.execution.event/v1`. EventKind remains the authoritative fact; payload
data carries audit details such as provider receipt, observations, errors, and
recovery authorization.

### Adapter behavior

Adapters expose orthogonal statuses directly. A transport-level success does
not become physical success, and an operation error does not overwrite known
dispatch evidence. Recovery is a separate operation rather than an implicit
retry flag on Execute.

## Alternatives

### Put orchestration in core

Rejected. Core would acquire runtime, storage, and workflow responsibilities
that can evolve independently.

### Hold a database transaction across dispatch

Rejected. SQLite cannot atomically roll back a physical operation, and a long
transaction increases contention without closing the uncertainty gap.

### Retry the same Attempt ID

Rejected. An Attempt identifies one possibly executed operation. Re-entry can
duplicate physical effects while erasing the distinction between attempts.

### Automatically recover incomplete attempts

Rejected. A process may have stopped after the physical action but before its
dispatch fact was committed. Incomplete means inspect, not safe to replay.

## Drawbacks

Callers must manage unique identities and make recovery decisions explicitly.
The SQLite Journal adds an infrastructure dependency. A post-I/O Journal outage
can still leave durable history behind in-memory knowledge; this limitation is
reported but cannot be eliminated by a local transaction.

## Open questions

- Which actor identity and signature format should production recovery
  authorization use?
- Should an outbox replicate local facts to optional cloud storage?
- Which authentication mechanism should a future MCP transport standardize?

## Decision

Accepted. Application coordination lives in `execution`, adapters depend on its
consumer-defined interfaces, and physical ambiguity always requires explicit
recovery.
