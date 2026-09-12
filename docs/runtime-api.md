# Local runtime and HTTP API

`cmd/habio-server` is the reference composition root. It wires the HTTP adapter
to application use cases, a durable SQLite Journal, and the Home Assistant
anti-corruption layer. Core imports none of these packages.

## Configuration

Required environment variables:

| Variable | Meaning |
| --- | --- |
| `HABIO_HOME_ASSISTANT_URL` | Absolute local Home Assistant HTTP(S) URL. |
| `HABIO_HOME_ASSISTANT_TOKEN` | Home Assistant long-lived access token. |
| `HABIO_HOME_ASSISTANT_BINDINGS` | JSON object mapping logical target names to entity IDs. |

Optional variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `HABIO_LISTEN` | `127.0.0.1:8080` | HTTP listen address. |
| `HABIO_DATABASE` | `habio.db` | SQLite file path. |
| `HABIO_API_TOKEN` | empty on loopback | Bearer token for every endpoint except health. Required on a non-loopback address. |
| `HABIO_PROVIDER_TIMEOUT` | `10s` | One Home Assistant request timeout. A timeout remains dispatch-unknown. |
| `HABIO_VERIFICATION_MAX_AGE` | `30s` | Maximum accepted Home Assistant observation age. |

Example local launch:

```sh
export HABIO_HOME_ASSISTANT_URL=http://home-assistant.local:8123
export HABIO_HOME_ASSISTANT_TOKEN=replace-with-local-token
export HABIO_HOME_ASSISTANT_BINDINGS='{"living-room-light":"light.living_room"}'
go run ./cmd/habio-server
```

The reference runtime uses an allow-all Admitter after HTTP authentication. It
is suitable for a controlled proof of concept, not a production authorization
policy. Install a policy-specific Admitter at the composition root before
exposing physical actions to multiple users. When listening beyond loopback,
terminate TLS at a trusted local reverse proxy; a bearer token does not protect
plaintext transport.

## Execute

`POST /v1/actions/execute` requires caller-generated immutable Action and
Attempt IDs:

```json
{
  "action": {
    "id": "action-20260912-001",
    "target": "living-room-light",
    "name": "turn_on",
    "input": {"brightness_pct": 40},
    "requested_at": "2026-09-12T12:00:00Z"
  },
  "attempt_id": "attempt-20260912-001"
}
```

A semantic result is returned even when the software path also returned an
error:

```json
{
  "action_id": "action-20260912-001",
  "attempt_id": "attempt-20260912-001",
  "admission": "admitted",
  "dispatch": "unknown",
  "effect": "unknown",
  "provider": "homeassistant",
  "operation_error": "habio execution dispatch: context deadline exceeded"
}
```

This response is not permission to retry. Submit the same Attempt ID again and
the server returns `409 Conflict` without provider I/O. A different Attempt ID
is also not an implicit safe retry; use explicit recovery.

## Verify

`POST /v1/actions/verify` accepts the same body shape as Execute. It obtains a
fresh observation and applies the configured Verifier without dispatching:

```json
{
  "action": {
    "id": "action-20260912-001",
    "target": "living-room-light",
    "name": "turn_on",
    "input": {"brightness_pct": 40},
    "requested_at": "2026-09-12T12:00:00Z"
  },
  "attempt_id": "attempt-20260912-001"
}
```

The result is `verified`, `unsatisfied`, or `inconclusive`, with observation IDs
when evidence exists. Verification never changes dispatch knowledge.

## Inspect

- `GET /v1/attempts/{attempt_id}` rebuilds admission, dispatch, effect, and
  verification from facts and exposes conflict flags.
- `GET /v1/recovery/incomplete` lists claimed attempts that have no conclusive
  dispatch fact. The endpoint is read-only and never retries them.
- `GET /healthz` is an unauthenticated process liveness check.

## Explicit recovery

`POST /v1/actions/recover` requires the exact original Action plus a new Attempt
ID and explicit authorization:

```json
{
  "action": {
    "id": "action-20260912-001",
    "target": "living-room-light",
    "name": "turn_on",
    "input": {"brightness_pct": 40},
    "requested_at": "2026-09-12T12:00:00Z"
  },
  "attempt_id": "attempt-20260912-recovery-001",
  "previous_attempt_id": "attempt-20260912-001",
  "authorized_by": "operator@example.com",
  "authorized_at": "2026-09-12T12:05:00Z",
  "reason": "operator inspected the device and approved another attempt"
}
```

The Journal rejects changed Action contents under the same Action ID. Recovery
authorization and the link to the prior Attempt are committed before provider
I/O.

## Transport boundary

The HTTP adapter is the current executable entry point. A future MCP adapter
should call the same application use cases and preserve the same orthogonal
statuses; MCP is not a core dependency and is not implemented in this runtime.
