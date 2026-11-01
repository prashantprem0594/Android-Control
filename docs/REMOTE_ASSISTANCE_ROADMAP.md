# Remote assistance roadmap

## Current implementation

The repository currently provides a consent-based remote-assistance path:

- Child screen sharing starts only after the child user approves Android's MediaProjection prompt.
- A persistent foreground notification lets the child stop sharing.
- Parent screen viewing uses authenticated screenshot/frame requests.
- Parent controls include tap, swipe, long press, Back, Home, and Recents.
- Child execution requires the explicitly enabled Accessibility service.
- Secure windows, DRM-protected content, and Android system consent cannot be bypassed.

Focused Android compilation and server command tests passed; whitespace validation is clean.

## Implemented session infrastructure

The repository now also contains short-lived authenticated signaling-session persistence and
offer/answer handoff routes. Sessions are single-use per device, expire automatically, can be
closed by the parent, and use device credentials for the child answer/ICE side. This is signaling
metadata only; it does not carry media.

## Deferred AnyDesk-grade work

True smooth, low-latency remote video is intentionally deferred. The current repository has no
WebRTC client or TURN deployment. A production implementation
needs:

1. Android WebRTC dependencies and a MediaProjection video track.
2. Parent WebRTC rendering and session controls.
3. Authenticated offer/answer/ICE signaling with short-lived session tokens.
4. TURN credentials and a deployed TURN relay for mobile/NAT reliability.
5. Session expiry, disconnect, revocation, auditing, and reconnect handling.

The signaling portion of item 3 is implemented. The remaining client code can be written in this
repository, but it cannot be validated as a reliable cross-network product until signaling and TURN
are deployed. Item 4 is
deployment work, not an Android-only code change.

OEM-specific full-control plugins cannot be implemented generically here. Some devices require
manufacturer-signed input-control support; that requires the device vendor or an approved plugin.
Android secure-window restrictions and per-session screen-capture consent remain mandatory.

## Rough feasibility estimate

| Area | Implementable now | Remaining dependency |
| --- | ---: | --- |
| Consent + visible screen-share lifecycle | 100% | None beyond device testing |
| Generic gestures and system navigation | 100% | Accessibility permission |
| Parent session UI and authenticated command flow | 80% | Final UX/device testing |
| WebRTC client and signaling code | 70-80% | Android WebRTC dependency and integration testing |
| Reliable cross-network media delivery | 30-40% | Deployed TURN/signaling infrastructure |
| OEM-specific full input control | 0% generically | Manufacturer/plugin support |
