CREATE TABLE pairing_codes (
    code_hash BLOB PRIMARY KEY NOT NULL CHECK (length(code_hash) = 32),
    parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    enrollment_id TEXT
) STRICT;

CREATE TABLE devices (
    id TEXT PRIMARY KEY NOT NULL,
    parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL,
    public_key_der BLOB NOT NULL,
    key_algorithm TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'active', 'revoked')),
    created_at TEXT NOT NULL,
    enrolled_at TEXT,
    revoked_at TEXT,
    revocation_reason TEXT
) STRICT;

CREATE TABLE device_enrollments (
    id TEXT PRIMARY KEY NOT NULL,
    device_id TEXT NOT NULL UNIQUE REFERENCES devices(id) ON DELETE CASCADE,
    challenge_hash BLOB NOT NULL CHECK (length(challenge_hash) = 32),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    completed_at TEXT,
    verification_attempts INTEGER NOT NULL DEFAULT 0 CHECK (verification_attempts >= 0)
) STRICT;

CREATE INDEX pairing_codes_parent_idx ON pairing_codes(parent_account_id);
CREATE INDEX devices_parent_idx ON devices(parent_account_id);
