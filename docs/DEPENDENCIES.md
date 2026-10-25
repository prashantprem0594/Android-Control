# External dependency register

This register distinguishes source-code libraries from hosted services. A library is built into software we control; it does not create a paid account or runtime service dependency.

| Dependency | Pinned version | Type | Runtime account or fee | Purpose and exit path |
|---|---:|---|---|---|
| `modernc.org/sqlite` | 1.61.0 | BSD-3-Clause Go library | None | Pure-Go SQLite driver with no C compiler or database server. It can be replaced behind `database/sql`; data remains a standard SQLite file. |
| `golang.org/x/crypto` | 0.58.0 | BSD-3-Clause Go library | None | Supplies the Argon2id password-hashing primitive compiled into our server. It has no hosted component or account. |
| Gradle | 9.6.0 | Build tool | None | Reproducible Android builds through the checked-in wrapper. |
| Android Gradle Plugin | 9.4.0 | Build plugin | None | Compiles Android applications against API 37. |
| Firebase Cloud Messaging | HTTP v1 adapter implemented; live integration pending | Hosted push transport | FCM transport only | Random opaque sync hints only. Disabled by default until OAuth/registration sources and Android Firebase configuration exist; foreground and scheduled HTTPS sync operate independently. |

All Go transitive module versions and checksums are pinned by `server/go.mod` and `server/go.sum`.
