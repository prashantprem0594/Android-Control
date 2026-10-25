# Architecture baseline

## Principles

1. The backend database is the source of truth for commands and policies.
2. FCM is an optional wake-up transport, never the command store.
3. Every remote action is authenticated, authorized, expiring, idempotent, and audited.
4. Desired policies survive offline operation; transient commands do not live forever.
5. The child application is visibly enrolled and call activity remains visible.
6. Emergency calling and a tested recovery path are never blocked.

## Components

### Parent Android application

- Native Kotlin and Jetpack Compose.
- Authenticates to the self-hosted backend.
- Creates commands and desired policies.
- Receives FCM notifications and active-session WebSocket updates.
- Displays confirmed device state separately from requested state.

### Child Android application

- Native Kotlin and Jetpack Compose.
- Device Owner / device policy controller.
- Default dialer with a complete `InCallService` implementation.
- Local VPN policy engine.
- Local Room database for commands and policies.
- Android Keystore device identity.
- FCM receiver followed by authenticated HTTPS synchronization.

### Control server

- Go application exposing HTTPS and WebSocket endpoints.
- Own parent authentication and device enrollment.
- Durable commands, policies, audit history, and APK release metadata.
- FCM provider adapter using the HTTP v1 API.
- SQLite for the personal-use deployment; PostgreSQL remains a future migration option.

## Command lifecycle

```text
QUEUED -> DELIVERED -> SUCCEEDED
                    -> FAILED
QUEUED -> EXPIRED
```

FCM delivery does not advance a command to `DELIVERED`. That transition occurs only after the child downloads and acknowledges the command.

## Call-answer sequence

```text
Child Telecom reports RINGING
  -> backend stores call event
  -> parent receives visible FCM notification
  -> parent submits ANSWER_CALL
  -> backend stores expiring command
  -> FCM sends SYNC_REQUIRED to child
  -> child downloads command
  -> child verifies the same call is still ringing
  -> child calls Call.answer(audio-only)
  -> child requests speaker endpoint
  -> child acknowledges outcome
```

## Firebase boundary

Only Firebase Cloud Messaging is permitted. The Firebase project stays on the Spark plan without a billing account.

Prohibited Firebase services:

- Authentication
- Firestore
- Realtime Database
- Cloud Functions
- Storage
- Hosting
- Remote Config
- Phone authentication

FCM messages contain an opaque reason and optional command identifier. Sensitive data remains on the self-hosted backend.

