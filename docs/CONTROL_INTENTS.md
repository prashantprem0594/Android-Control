# Audited command and policy intent

These parent-authenticated routes persist requested actions. Device fetch and
acknowledgement are documented in [the command channel](COMMAND_CHANNEL.md).
Device policy prototypes and physical verification requirements are documented
in [device policies](DEVICE_POLICIES.md).

- `POST /v1/devices/{deviceID}/commands`: body
  `{"type":"LOCK_DEVICE","ttlSeconds":60}`. Returns 201 with a generated
  UUID, version 1, device ID, type, empty payload, issue/expiry timestamps, and
  `status: "queued"`. TTL is 1–3600 seconds. Other command types are rejected
  until their execution contracts are implemented. Each POST creates a new
  command unless the optional `Idempotency-Key` header is supplied; use that
  header to retry an ambiguous response safely.
- `GET /v1/devices/{deviceID}/commands/{commandID}`: inspect persisted command
  intent and status. The owner can inspect history after device revocation.
- `PUT /v1/devices/{deviceID}/policy`: body
  `{"expectedVersion":0,"internetPaused":true,"blockedPackages":["org.example.app"]}`.
  Returns the new desired policy version. All three fields are required.
  Version 0 means no stored policy; a stale version returns 409. Every successful
  PUT increments the version, including a repeated desired state.
- `GET /v1/devices/{deviceID}/policy`: read desired state; an active device with
  no stored policy returns version 0, internet unpaused, an empty package list,
  and a zero update timestamp.

Parent PUTs also accept `alwaysAllowedPackages` and `schedules`; see
[offline schedule semantics and format negotiation](OFFLINE_SCHEDULES.md).
Parent GETs expose desired and reported effective-state keys separately.

Writes and policy reads require an active device owned by the authenticated
parent. Missing, revoked, or other-account devices return 404. Command-history
reads also verify ownership. Authentication is required on every route. Invalid
input returns 400; unexpected storage failures return a generic 500.

Package lists allow at most 100 distinct package names, each at most 255
characters, with Java identifier segments separated by dots. Android, Google
Android, and App Control namespaces are excluded. Actual device enforcement
must additionally discover and preserve all critical Redmi/HyperOS packages;
these server checks do not replace the device inventory and recovery checks.

Migration 0007 stores commands and desired policies. Its triggers append
`command_created` and `policy_updated` audit events in the same transaction as
the corresponding write. Audit events contain parent attribution, time, and a
command/device target ID. They exclude command payloads and policy contents.
An audit failure rolls back the write. Ownership and active state are checked
inside the write transaction; optimistic policy versions prevent lost updates.

Tests cover restart persistence, cross-parent rejection, revoked devices,
simultaneous policy updates, invalid input, audit inspection, and injected audit
failures. Command transport, lazy expiry and child lock sync now exist;
physical-device verification and production hardening remain pending. A queued
record and a saved policy never imply confirmed device success.
