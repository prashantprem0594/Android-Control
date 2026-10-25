# Remote call command API

Remote call actions are short-lived commands for an already-live Telecom call.
They only submit answer, reject, or end requests; they do not expose call
content, place calls, or report success merely because Telecom accepted a
request. The child reports success only after observing the requested state
change for the exact in-memory call identity. Actions use the foreground
parent call-controls screen. An optional bounded
[parent incoming-call watch](PARENT_CALL_ALERTS.md) provides review-only alerts
that fetch fresh exact-call state before offering commands. Continuous parent
push, hardware and enrolled-device acceptance remain open.

## Foreground call discovery

The signed-in parent app's **Foreground calls** screen reads
`GET /v1/devices/{deviceID}/calls`. The child publishes a bounded snapshot
promptly after call arrival, state change, or removal, plus a coalesced
15-second refresh while calls remain active. These are lightweight snapshot
requests; they do not run policy or inventory synchronization.

Before reporting, the child obtains a server-monotonic sequence:

```http
POST /v1/device/calls/sequence
Authorization: Bearer <device-session>
```

The response is `{"sequence":42}`. The snapshot includes that sequence:

```http
POST /v1/device/calls
Authorization: Bearer <device-session>
Content-Type: application/json
```

```json
{
  "calls": [
    {
      "callId": "a3d7a190-8503-4bf4-9c12-123456789abc",
      "state": "RINGING",
      "remoteActionsAllowed": true
    }
  ],
  "sequence": 42
}
```

The GET response contains server-authored `serverTime`; a reported snapshot
also includes `observedAt` and `snapshotSequence`. The only per-call fields
are the opaque canonical UUID v4 `callId`, one of
`RINGING`, `ACTIVE`, `HOLDING`, `DIALING`, `CONNECTING`,
`SELECTING_ACCOUNT`, `DISCONNECTING`, `DISCONNECTED`, or `OTHER`. Empty
`calls` clears the snapshot. The parent compares `observedAt` with `serverTime`,
treats snapshots older than 30 seconds as stale, and does not offer action
buttons. Each call also has `remoteActionsAllowed`; it must be false for
emergency, unclassified, or build-disabled calls. The parent disables action
buttons when this is false. The server also requires the exact call to be
present in a snapshot no older than 30 seconds with permission true and a
compatible reported state before accepting a command. The child rechecks the
live in-memory Telecom call and state when executing it.

Snapshots are not streamed continuously. Refreshing the parent's screen only
reads the last snapshot. On process/service restart the child clears old
in-memory call IDs and publishes a fresh snapshot; server sequence ordering
prevents a delayed older report from replacing a newer one. Call IDs are
process-local. The parent screen shows only call IDs and states, never caller
details. The foreground snapshot worker and regular command/policy sync reuse
one expiring in-memory device session. Authentication and each API request are
serialized through a short session lock so one worker cannot revoke another's
in-flight request, without holding snapshot publication behind an entire
policy sync. The bearer token is not persisted or logged.

## Feature flags

The server's `APP_CONTROL_REMOTE_CALLS_ENABLED` setting defaults to `false`.
When false, call commands are rejected and snapshots cannot authorize remote
actions; lock and policy controls are unchanged. The child has the matching
Gradle property `appControlRemoteCallsEnabled`, also defaulting to `false`;
call commands received by a disabled build are acknowledged failed without
executing a Telecom action. Enable both explicitly for a controlled test.

## Parent: queue an action

```http
POST /v1/devices/{deviceID}/commands
Authorization: Bearer <parent-access-token>
Content-Type: application/json
Idempotency-Key: <optional 16-128 character key>
```

The authenticated parent must own the active device. A different parent's
request returns `404 device_not_found`. Call actions accept a lifetime of
1–30 seconds; a longer or zero lifetime is rejected with `400 invalid_request`.
Use the exact opaque call ID assigned to the current Telecom call by the child.
It is a canonical lowercase UUID v4 and is never resolved to a different call.
IDs are process-local and published only through the bounded call-state
snapshot above.

```json
{
  "type": "ANSWER_CALL",
  "callId": "a3d7a190-8503-4bf4-9c12-123456789abc",
  "ttlSeconds": 20
}
```

`type` is exactly one of `ANSWER_CALL`, `REJECT_CALL`, or `END_CALL`.
`callId` is required, must be a canonical lowercase UUID v4, and is the only
call-related payload field.
`ttlSeconds` is an integer from 1 through 30. Additional fields—including
phone numbers, contact names, or call content—are rejected. `LOCK_DEVICE`
continues to use its existing empty payload and TTL range.

The `201` response is a queued command envelope (`commandId`, `deviceId`,
`type`, `payload`, `issuedAt`, `expiresAt`, and `status`). `queued` means only
that the intent was durably stored. Idempotent retries with the same key and
same action, call ID, and lifetime return the original command. Reusing a key
with different input returns `409 idempotency_conflict`.

## Child: fetch and acknowledge

The enrolled child uses its signed device session:

- `GET /v1/device/commands` fetches at most 50 queued/delivered commands.
- `POST /v1/device/commands/{commandID}/ack` accepts
  `{"result":"succeeded"}` or `{"result":"failed"}`.
- `GET /v1/devices/{deviceID}/commands/{commandID}` lets the owning parent
  inspect the durable result.

Delivery changes status to `delivered`, not success. Only the authenticated
target device can acknowledge. The child rejects expired commands, missing or
stale call IDs, emergency/protected calls, and actions incompatible with the
current state. Answer and reject require `RINGING`; end requires `ACTIVE` or
`HOLDING`. Telecom request submission alone is not success: answer requires an
observed `ACTIVE` state, while reject/end require the same call to be observed
`DISCONNECTED` or removed; `DISCONNECTING` alone is not completion. If that
result is not observed by the command deadline, the child acknowledges
`failed`. An interrupted command is not repeated after process recovery.

## Data handling and verification scope

The call snapshot API contains only opaque call IDs and Telecom states. The
command API, stored payload, and command history contain only the opaque
call ID and action. Request logging records route templates and never request
bodies; audit events record command identity/status but not payload. The child
does not add number, contact name, or call content to command reports. FCM
remains exactly one opaque `syncHint`; no call ID, action, or command data is
sent through FCM.

Focused server authorization, payload, idempotency, and expiry checks and JVM
call identity/state/deadline/outcome checks are run through the project test
logging wrapper. They are not physical call evidence. Keep Redmi, SIM, Telecom
and enrolled end-to-end hardware acceptance gates open until tested.
