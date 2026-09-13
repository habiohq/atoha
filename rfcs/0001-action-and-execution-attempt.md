# RFC 0001: Action and ExecutionAttempt

- Status: Accepted
- Authors: Atoha maintainers
- Created: 2026-09-06
- Related issues: #2

## Summary

Represent immutable intent as Action and each execution of that intent as a
separately identified ExecutionAttempt. A recovery attempt may link to the
prior attempt it recovers, but this relationship makes no replay-safety claim.

## Motivation

One physical intent can have more than one attempt after an ambiguous result.
Using one identity for both loses the distinction between deduplicating a
request and repeating a physical operation. Atoha needs both identities to
record recovery without rewriting history.

## Proposal

Action contains a caller-supplied ActionID, logical target, operation name,
opaque input bytes, and requested time. It is created through a validating
constructor and exposes no mutable internal data. Natural language, provider
endpoint identity, device type, and provider-specific metadata are absent.

ExecutionAttempt contains a caller-supplied AttemptID, ActionID, start time, and
an optional RecoveryOf AttemptID. Each actual provider dispatch uses one
attempt. A recovery receives a new AttemptID.

IDs are opaque non-empty strings. Core does not generate UUIDs because identity
generation and persistence belong to the runtime. Required timestamps are
caller supplied so tests and event reconstruction remain deterministic.

The zero values of Action and ExecutionAttempt are invalid. Constructors reject
missing fields and identifiers with surrounding whitespace. Input bytes are
copied on construction and access.

## Alternatives

### Public structs

Public fields make decoding easy but permit mutation after admission or
dispatch. That weakens the meaning of Action identity, so v0.1 uses private
fields and accessors.

### One ID for Action and attempt

This makes the first call convenient but cannot distinguish an immutable intent
from repeated physical effects. It is rejected.

### Attempt sequence number

A sequence number implies centralized ordering and can conflict under
concurrent recovery. Explicit opaque identity and an optional recovery link are
sufficient for v0.1.

### Core-generated IDs and timestamps

Convenient constructors that read randomness and the system clock make the
core less deterministic and complicate persistent runtimes. The caller owns
generation.

### Generic metadata map

An unrestricted map risks becoming an unreviewed semantic extension mechanism.
It is deferred until a concrete cross-provider requirement appears.

## Drawbacks

Private fields require explicit serialization at adapter or storage boundaries.
Opaque bytes require the caller and provider to agree on an encoding. Caller-
supplied identities and timestamps add work for runtimes.

## Open questions

- Does a later conformance format require a declared media type for Input?
- Which correlation data is common enough to add without a metadata escape
  hatch?
- Should a recovery record a structured reason in a later event rather than in
  ExecutionAttempt?

## Decision

Accepted for v0.1. Add metadata, generated identity, or sequence semantics only
through a later RFC supported by at least two implementations.
