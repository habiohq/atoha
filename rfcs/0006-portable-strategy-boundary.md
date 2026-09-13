# RFC 0006: Portable Strategy boundary

- Status: Accepted
- Authors: Atoha maintainers
- Created: 2026-09-06
- Related issues: #8

## Summary

Define portability through package-local logical requirements, data and setting
schemas, declared Action permissions, external installation bindings, explicit
approval, and an immutable deterministic artifact. Keep the runtime format,
global taxonomy, conflict policy, and marketplace outside this decision.

## Motivation

A Strategy containing Home Assistant entity IDs, vehicle identifiers, vendor
accounts, or household-specific data cannot move between installations. A
Strategy that dispatches directly bypasses Atoha's admission and execution
evidence. Running an LLM for every control decision weakens determinism and
auditability.

The contract must preserve portability without asking Atoha core to standardize
all physical capabilities.

## Proposal

A portable package declares identity/version, package-local logical
requirements, external data inputs, user settings, Action permissions,
provenance, and an immutable artifact digest. Requirement and schema names are
local to the package; ecosystem conventions can emerge without becoming core
types.

Installation-local bindings map logical requirements to Resolver targets and
data-source implementations. Bindings and credentials never enter the portable
package.

AI-generated Strategies are drafts. Schema validation, simulation, permission
review, binding review, and explicit user approval precede execution. The
approved immutable artifact is evaluated deterministically and produces Action
proposals. The application, not the Strategy, assigns Action identity, handles
admission/conflicts, and asks Atoha to execute.

The serialization example in `docs/strategy-contract.md` is non-normative. A
runtime and sandbox format require a later evidence-backed RFC.

Overlapping Action permissions require rejection or an explicit application
policy until resource-claim and conflict semantics are accepted separately.

## Alternatives

### Vendor-specific identifiers in packages

Simple for one household but not portable and risks credential leakage.
Rejected.

### Universal Atoha capability taxonomy

This competes with hardware platforms and grows core with every vertical.
Rejected. Vocabulary remains package-local until interoperable conventions are
proven.

### Strategy dispatches directly

This bypasses admission, conflict handling, and the execution fact boundary.
Rejected.

### LLM in every runtime decision

This makes identical inputs non-deterministic and complicates simulation and
audit. AI authors a draft; the approved runtime artifact executes.

### Choose WASM now

WASM may be useful, but choosing it before testing host capabilities,
sandboxing, time, I/O, and portability would prematurely fix the ecosystem
contract. Deferred.

### Numeric priority in v0alpha1

Priority without ownership and safety provenance turns conflicts into hidden
policy. Deferred to a conflict RFC.

## Drawbacks

Package-local vocabulary reduces immediate plug-and-play compatibility.
Installation binding and approval add setup work. Deferring a runtime format
means the document defines boundaries rather than an executable SDK.

## Open questions

- Which deterministic artifact formats satisfy sandbox and portability needs?
- How are ordered input snapshots represented for reproducible evaluation?
- What resource-claim model works across homes, energy systems, and robotics?
- Which compatibility and measured-outcome metrics can be shared privately and
  with meaningful consent?
- How should package-local schemas converge into optional ecosystem profiles?

## Decision

Accepted as the architectural boundary, not as a frozen wire format. No
Strategy type is added to the execution core. Runtime, conflict, marketplace,
and conformance work requires separate RFCs and implementations.
