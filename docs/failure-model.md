# Failure and uncertainty model

## The central distinction

Software-path failure and physical outcome are different facts.

```text
request timeout
  -> software path did not return a conclusive response
  -/-> physical action failed
```

The command may not have arrived, may have been accepted but not executed, may
have executed without changing the world as expected, or may have executed with
only its response lost. When available evidence cannot distinguish those cases,
Atoha reports an unknown outcome.

## Knowledge dimensions

The v0.1 model should preserve, without necessarily encoding as one enum:

1. **Admission:** was execution rejected or allowed?
2. **Dispatch:** is it known that a provider call was attempted or sent?
3. **Acknowledgement:** what did the provider claim to accept?
4. **Observation:** what source observed what value, and when?
5. **Verification:** did a named verifier find sufficient evidence for the
   desired effect?
6. **Confidence boundary:** which of those facts remain unknown?

`accepted`, `dispatched`, `acknowledged`, `observed`, and `verified` are not
necessarily a single monotonic sequence. For example, an independent sensor can
observe the desired state when a provider acknowledgement was lost.

## Representative cases

| Case | Honest report | Unsafe simplification |
| --- | --- | --- |
| Admitter denies action before dispatch | Rejected; not dispatched | Provider failure |
| Provider returns validation error before sending | Known failure; not dispatched | Physical failure after attempt |
| Provider accepts, no observation capability exists | Acknowledged; unverified | Verified success |
| Request times out after possible dispatch | Unknown/ambiguous | Failed, then retry |
| Fresh sensor contradicts desired state | Observed unsatisfied; verification failed | Transport error |
| Desired state is observed through an independent path | Verified with evidence | Merely acknowledged |
| Only stale desired state is available | Unverified or unknown under freshness policy | Verified success |

## Errors in Go APIs

A Go `error` can communicate transport failure, cancellation, invalid input, or
an internal defect. A result value must separately retain any known receipt,
observation, and outcome information. Callers must not need to infer physical
failure from `err != nil` or physical success from `err == nil`.

Cancellation is also not proof that a physical action was cancelled. The API
must preserve evidence received before context cancellation.

## Retry and replay

Atoha does not silently retry an ambiguous physical action. A recovery policy
needs evidence about protocol, provider, device, and world-state idempotency.

Every repeated attempt receives its own attempt identity and refers to the same
Action only when it is truly a recovery of that intent. The event history must
make the caller's decision visible.

The initial API should avoid a boolean `idempotent` flag unless its scope is
explicit. A structured replay assessment may be needed, but real provider cases
should drive that RFC.

## Open questions for the Outcome RFC

- Which terminal states are facts, and which are projections?
- Should dispatch, physical, and verification results be orthogonal types?
- How is contradictory evidence represented without deleting earlier facts?
- Which timestamps are required to evaluate observation freshness?
- How much provider-specific evidence can be retained without coupling core?
- How does a caller explicitly record a recovery decision?

RFC 0005 answers the storage direction for v0.1: retain immutable facts and
derive current knowledge through replaceable projections. Contradictory facts
remain in the log and cause the reference projection to expose an unknown,
conflicted dimension rather than selecting a convenient certainty.
