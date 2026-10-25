CREATE TABLE device_auth_challenges (
    id TEXT PRIMARY KEY NOT NULL,
    device_id TEXT NOT NULL UNIQUE REFERENCES devices(id) ON DELETE CASCADE,
    challenge_hash BLOB NOT NULL CHECK(length(challenge_hash) = 32),
    expires_at TEXT NOT NULL,
    consumed INTEGER NOT NULL DEFAULT 0 CHECK(consumed IN (0, 1))
) STRICT;

CREATE TABLE device_sessions (
    token_hash BLOB PRIMARY KEY NOT NULL CHECK(length(token_hash) = 32),
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL
) STRICT;
CREATE INDEX device_sessions_device_idx ON device_sessions(device_id);
