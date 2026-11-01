ALTER TABLE devices ADD COLUMN credential_version INTEGER NOT NULL DEFAULT 1 CHECK(credential_version > 0);
ALTER TABLE devices ADD COLUMN credential_updated_at TEXT;
ALTER TABLE devices ADD COLUMN last_reattached_at TEXT;

CREATE TABLE device_recovery_bindings (
    device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    binding_hash BLOB NOT NULL UNIQUE CHECK(length(binding_hash) = 32),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE device_recovery_requests (
    id TEXT PRIMARY KEY NOT NULL,
    device_id TEXT REFERENCES devices(id) ON DELETE CASCADE,
    request_token_hash BLOB NOT NULL UNIQUE CHECK(length(request_token_hash) = 32),
    proposed_public_key_der BLOB NOT NULL,
    key_algorithm TEXT NOT NULL,
    challenge_hash BLOB NOT NULL CHECK(length(challenge_hash) = 32),
    state TEXT NOT NULL CHECK(state IN ('awaiting_proof','awaiting_parent','approved','rejected')),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    proof_verified_at TEXT,
    resolved_at TEXT
) STRICT;
CREATE INDEX device_recovery_requests_device_idx ON device_recovery_requests(device_id,state);
CREATE INDEX device_recovery_requests_expiry_idx ON device_recovery_requests(expires_at,state);
