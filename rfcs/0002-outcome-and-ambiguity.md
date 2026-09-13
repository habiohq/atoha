# RFC 0002: Outcome and ambiguity

- Status: Accepted
- Authors: Atoha maintainers
- Created: 2026-09-06
- Related issues: #3

## Summary

Represent Outcome as three independent dimensions: admission knowledge,
dispatch knowledge, and physical-effect knowledge. The fully unknown Outcome is
the valid zero value. Go errors remain a separate return channel.

## Motivation

A single success/failure enum creates a false linear lifecycle. Provider
acknowledgement can exist without physical evidence, an observation can arrive
after a response timeout, and a transport error does not prove physical
failure. Atoha must retain partial and conflicting knowledge.

## Proposal

AdmissionStatus is unknown, rejected, or admitted. DispatchStatus is unknown,
not-dispatched, dispatched, or acknowledged. EffectStatus is unknown,
unverified, observed-satisfied, or observed-unsatisfied.

The statuses are an immutable projection, not the fact log itself. Their zero
values mean unknown. Outcome exposes each dimension but has no `Success`,
`Failed`, or implicit retry method.

Unknown and unverified are distinct. Unknown means evidence cannot establish
the relevant result, such as a timeout around possible dispatch. Unverified
means dispatch knowledge exists but sufficient observation/verification is not
available. The Observation and Verification RFC will refine the evidence that
can justify the observed statuses.

Constructors validate enum ranges but do not reject apparently contradictory
dimensions. Independent subsystems can disagree, and rejecting the projection
would discard evidence. The event/projection model must later make conflicts
visible.

Operations return `(Outcome, error)` when both physical knowledge and a Go
transport or implementation error matter. Neither return value derives the
other.

## Alternatives

### One lifecycle enum

A list such as requested, dispatched, acknowledged, verified, failed, and
unknown implies a total order that does not exist. It cannot naturally retain
an independent observation after an acknowledgement is lost. Rejected.

### Success boolean plus error

This erases unknown and unverified states and encourages blind retries.
Rejected.

### Confidence score

A numeric confidence hides the source and meaning of evidence. It is not a
substitute for explicit facts and verification policy. Rejected.

### Reject contradictory combinations

This gives a tidy state machine but loses real disagreement between dispatch
and observation paths. Contradictions remain representable and will be handled
by projections.

## Drawbacks

Callers must inspect more than one value. Some combinations need explanatory
evidence to be useful. The distinction between unknown and unverified requires
careful adapter language.

## Open questions

- Should a later projection carry an explicit conflict marker?
- Which facts cause an effect to become unverified rather than unknown?
- Does known irreversible failure need a separate effect state, or is it a
  failed verification supported by observations?
- How should adapter schemas version the three dimensions?

## Decision

Accepted for v0.1. Outcome stays orthogonal and conservative. Observation,
Verification, Provider receipts, and event projection add evidence without
collapsing the dimensions.
