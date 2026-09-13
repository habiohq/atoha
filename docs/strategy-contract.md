# Portable Strategy contract

## Status and boundary

This document defines the portability boundary for a future Strategy layer. It
does not add Strategy concepts to the Atoha execution package and is not a
runtime, language, sandbox, scheduler, conflict engine, or marketplace format.

```text
data sources
    |
approved deterministic Strategy artifact
    |
typed Action proposals
    |
application admission and conflict handling
    |
Atoha execution core
    |
Provider
```

A Strategy decides what should happen. Atoha records what happened when an
approved Action crossed the physical execution boundary.

## Portable unit

A portable Strategy package must describe the following logical information
without containing identifiers from its author's environment.

### Identity

- stable package name within the publisher's namespace;
- package version;
- contract version; and
- immutable artifact digest.

### Logical requirements

Named slots describe roles needed by this Strategy, not global device classes.
Each slot includes a human explanation and the operation signatures the
Strategy intends to produce.

For example, `vehicle-charger` is local to one Strategy package. It does not
claim that Atoha has standardized every EV charger. Another Strategy can use a
different vocabulary and a catalog can later publish compatible conventions.

### Data inputs

Each external input has a local name, documented value schema, freshness
expectation, and whether missing data makes evaluation inapplicable or
inconclusive. Calendar, weather, tariff, occupancy, and sensor APIs remain data
source extensions.

### User settings

Settings have local names, schemas, defaults where safe, descriptions, and
validation rules. Defaults must not silently authorize physical actions.

### Action permissions

The package declares the logical slots and operation names it may propose. The
installed application's Admitter enforces the approved subset. A Strategy
cannot expand its own permissions after installation.

### Deterministic artifact

The installed package identifies immutable executable content by digest. The
runtime format is deliberately unspecified until sandboxing and portability
experiments compare viable implementations. Re-evaluating the same artifact
with the same ordered inputs and settings must produce the same Action
proposals.

### Provenance

Publisher, source, license, review status, and artifact digest are available to
the installer. Marketplace reputation is additional metadata, not execution
authority.

## Environment binding

Bindings are installation-local and stored separately from the portable
package:

```text
Strategy logical requirement
        |
installation binding
        |
Atoha Resolver / data source
        |
provider endpoint or external service
```

A binding can select a Home Assistant logical target, MHS control, external
forecast, or another implementation. The portable package never contains
values such as `climate.living_room`, a vehicle serial number, account ID, or
vendor token.

Binding validation checks only the Strategy's declared operation and data
schemas. It does not require Atoha core to own a universal capability taxonomy.

## Authoring and approval

AI may translate natural language into a Strategy draft, but the draft follows
the same path as human-authored content:

```text
natural language
    -> draft package
    -> schema validation
    -> simulation/backtest
    -> permissions and binding review
    -> explicit user approval
    -> immutable deterministic artifact
```

An LLM is not consulted for each runtime decision by default. Changing the
artifact, requested permissions, or material settings creates a new approval
decision.

## Runtime output

A Strategy evaluation produces zero or more Action proposals plus diagnostic
facts. It does not dispatch them. The application assigns Action identity,
performs admission and conflict handling, and then invokes Atoha.

Proposals refer to logical requirement slots. Installation binding resolves
them before dispatch. Strategy evaluation errors are not Atoha physical
outcomes.

## Conflicts and ownership

The portable contract does not define numeric priority or last-write-wins. Such
defaults would hide policy and can be unsafe when two Strategies act on one
resource.

Before concurrent Strategy execution is supported, a separate RFC must define:

- resource claims and their scope;
- exclusive versus cooperative ownership;
- explicit user and safety override;
- priority provenance;
- detection before dispatch; and
- audit facts for the chosen resolution.

Until then, a runtime must reject or require explicit application policy for
installations whose declared action permissions overlap.

## Privacy and marketplace data

Sharing source packages does not opt a user into sharing household data,
actions, savings, reliability metrics, or ratings. Outcome and compatibility
telemetry is opt-in, minimized, and separate from local execution.

## Non-normative example

This pseudo-manifest illustrates local names; it is not a frozen serialization
format:

```yaml
contract: atoha-strategy/v0alpha1
name: example.org/solar-first-charging
version: 0.1.0

requirements:
  - name: vehicle-charger
    operations:
      - set_charging_enabled

data:
  - name: next-day-solar-forecast
    schema: energy-forecast-defined-by-this-package
    freshness: 6h
  - name: next-day-travel-plan
    schema: travel-plan-defined-by-this-package
    on_missing: inconclusive

settings:
  - name: minimum-charge
    schema: percentage-defined-by-this-package

permissions:
  actions:
    - vehicle-charger.set_charging_enabled

artifact:
  digest: sha256:<immutable-content-digest>
  format: <runtime-selected-after-future-rfc>
```

An installation supplies all concrete bindings and approvals outside this
document.
