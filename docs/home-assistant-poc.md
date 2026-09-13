# Home Assistant proof of concept

## Purpose

The `provider/homeassistant` package tests Atoha's execution semantics against
Home Assistant without making Home Assistant part of core. It is intentionally
a proof, not a production integration or device model.

Internally it is an anti-corruption layer: `Provider` is the Atoha-facing
facade, `translator.go` owns entity/service conversion and binding validation,
and `client.go` owns Home Assistant REST DTOs and transport behavior.

The implementation follows Home Assistant's documented REST endpoints:

- `POST /api/services/<domain>/<service>` calls a service action and returns
  after Home Assistant executes it.
- `GET /api/states/<entity_id>` obtains Home Assistant's current entity state.
- Requests use bearer-token authentication and JSON.

References:

- <https://developers.home-assistant.io/docs/api/rest/>
- <https://developers.home-assistant.io/blog/2024/07/16/service-actions/>

Home Assistant notes that the service-action response contains states changed
during execution, including changes caused by something else. Atoha therefore
treats a successful response as Provider acknowledgement only and performs a
separate state observation and verification.

## Binding and actions

The PoC uses configuration to bind logical names to Home Assistant entity IDs:

```text
living-room-light   -> light.living_room
living-room-climate -> climate.living_room
living-room-tv      -> media_player.living_room_tv
```

Action Name becomes the Home Assistant service action. The domain is taken from
the resolved entity ID. Opaque Action input is a JSON service-data object; the
provider adds the resolved `entity_id` and rejects attempts to override it.

The fixtures exercise:

- `turn_on` for a light;
- `set_temperature` for a climate entity; and
- `turn_off` for a media-player entity.

These are provider fixtures. No Light, AirConditioner, or TV type exists in the
Atoha core.

## Failure semantics

| REST result | Dispatch knowledge | Physical knowledge |
| --- | --- | --- |
| Invalid local action/binding | Not dispatched | Unknown |
| HTTP client timeout/error | Unknown | Unknown |
| Home Assistant returns non-2xx | Dispatched, not acknowledged | Unknown |
| Home Assistant returns 2xx | Acknowledged with receipt | Unverified until observation |

The provider performs no automatic retry. A timeout after handing the request
to the HTTP client remains ambiguous. A 2xx status remains acknowledged even if
the response body cannot be read or exceeds the retention limit; the receipt
keeps every body byte available within that limit and the body problem is
returned separately as a Go error.

Before any HTTP request, Dispatch rejects an attempt belonging to another
Action. Dispatch and Observe also revalidate the resolved entity against the
Action's configured binding so that independently supplied values cannot be
swapped or forged into a different physical target.

## Observation and verification

Observer preserves Home Assistant `last_updated` as ObservedAt and records the
local ingestion time separately. StateVerifier demonstrates an explicit
freshness window for `turn_on`, `turn_off`, and `set_temperature` fixtures.

Tests cover fresh verified state, missing evidence, stale evidence, fresh
contradiction, numeric climate setpoint equality, and evidence supplied by an
independent source.

## Local usage boundary

The package needs only a local Home Assistant base URL and token. It has no
Atoha Cloud endpoint, account, or network dependency beyond the configured Home
Assistant instance.

The runnable composition root is `cmd/atoha-server`; configuration and request
examples are documented in [runtime-api.md](runtime-api.md).

The implementation remains in this repository during semantic discovery. It
should move only when its dependency/release cycle is independent enough to
justify `atoha-server` or `atoha-provider-homeassistant`; repository topology is
not used as a substitute for a stable contract.
