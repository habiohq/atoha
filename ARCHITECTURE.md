# Atoha architecture

## System boundary

```text
probabilistic caller
        |
   adapter (HTTP/MCP)
        |
        v
+-----------------------+
| application use cases |
| Execute / Verify      |
| Recover / Query       |
+-----------+-----------+
            |
     core contracts
            |
       provider contract
            |
   hardware platform
            |
     physical system
```

The adapters, storage, and providers are edges. The core knows neither HTTP,
MCP, SQLite, nor Home Assistant. Application use cases coordinate core ports
without moving workflow or infrastructure concepts into core.

## Logical components

### Core

Defines Actions, attempts, outcome knowledge, observations, verification input
and result, lifecycle facts, and narrow extension contracts. It does not
discover devices, interpret natural language, choose policy, or coordinate a
runtime workflow.

### Application

The `execution` package contains one use case per operation: Execute, Verify,
Recover, Get, and ScanIncomplete. It defines the stronger Journal port it needs.
Attempt and Action identity are committed before provider I/O; ambiguous or
incomplete attempts are never retried automatically.

### Adapters

Translate caller protocols into Actions and expose the resulting evidence. The
reference HTTP adapter offers execute, verify, explicit recovery, attempt query,
and incomplete-attempt inspection. A future MCP adapter should delegate to the
same use cases. Neither may claim physical success for unknown or unverified
results.

### Storage

The memory Journal is for tests and examples. The SQLite Journal durably
registers immutable Actions, claims unique Attempts, and appends fact groups in
canonical sequence. Storage does not decide semantic outcomes or recovery.

### Providers

Absorb protocol, vendor, and hardware-platform complexity. Home Assistant is the
first proof-of-concept provider because it already owns integration, entity,
and device concerns. MHS, ROS2, OpenHAB, and custom systems can be peer providers.

### Policy extensions

Resolvers, Admitters, Observers, Verifiers, and EventSinks attach through small
contracts. Concrete authorization and safety policy remains outside the core.

## Local-first deployment

```text
local app or local AI
        |
 self-hosted Atoha server
        |
 local provider (for example Home Assistant)
        |
     physical device
```

The execution path must work with no Atoha cloud dependency. Cloud products may
add managed connectivity and operations but cannot be a prerequisite for local
execution.

## Repository architecture

Start with one repository, not the final ecosystem topology.

### `habiohq/atoha` (now)

- core Go library and provider-independent semantics;
- application execution use cases;
- local SQLite Journal and HTTP runtime;
- design documents and RFCs; and
- early conformance examples.

The reference server and Home Assistant proof of concept are currently developed
alongside core to test the contracts. Package dependency checks prevent them
from being imported inward.

### `habiohq/atoha-server` (when runtime work starts to move independently)

- self-hosted runtime and CLI;
- local API, configuration, and storage;
- MCP adapter and reference UI; and
- initially, the Home Assistant provider.

### Later repositories

Provider, MCP, strategy, and conformance repositories are created only when an
independent release cycle or maintainer boundary is demonstrated. Likely
candidates include `atoha-provider-homeassistant`, `atoha-provider-mhs`,
`atoha-mcp`, `atoha-strategy-sdk`, and `atoha-conformance`.

The split rule is:

> Independent release cycle means separate repository.

An implementation that a third party can replace is also a candidate to live
outside core, but replaceability alone does not require an immediate split.

## OSS and SaaS boundary

The product principle is **open execution, managed convenience**.

Apache-2.0 OSS includes the execution core, self-hosted runtime, provider
contracts, local MCP, local execution, local strategy runtime, and extension
SDKs. A proprietary Atoha Cloud may provide hosted MCP, OAuth, remote
connectivity, credential management, fleet operations, history, observability,
audit, policy UI, integration registry, marketplace, billing, teams, and
support.

Commercial value comes from managed operations, not from disabling local
execution or withholding its semantics.

## Dependency rules

- core must not import application, adapter, provider, storage, device-model, or cloud packages.
- application may depend on core and projections, not concrete adapters, providers, or storage.
- adapters may depend on application/core contracts, never concrete providers or storage.
- providers may depend on core contracts and vendor clients, never the reverse.
- storage may depend on core contracts and database drivers, not application or providers.
- strategy runtimes may produce Actions but cannot add strategy concepts to core.
- storage is behind an event/fact contract and is not the source of semantic
  truth.

These rules are enforced in CI by `scripts/check-dependencies.sh`.
