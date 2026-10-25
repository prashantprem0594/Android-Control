CREATE TABLE parent_accounts (
    id TEXT PRIMARY KEY NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE auth_login_failures (
    email TEXT PRIMARY KEY NOT NULL,
    failure_count INTEGER NOT NULL CHECK (failure_count >= 0),
    window_started_at TEXT NOT NULL,
    blocked_until TEXT,
    updated_at TEXT NOT NULL
) STRICT;

CREATE TABLE auth_session_families (
    id TEXT PRIMARY KEY NOT NULL,
    parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    revocation_reason TEXT
) STRICT;

CREATE TABLE auth_refresh_tokens (
    token_hash BLOB PRIMARY KEY NOT NULL CHECK (length(token_hash) = 32),
    family_id TEXT NOT NULL REFERENCES auth_session_families(id) ON DELETE CASCADE,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    replaced_by_hash BLOB CHECK (replaced_by_hash IS NULL OR length(replaced_by_hash) = 32)
) STRICT;

CREATE TABLE auth_access_tokens (
    token_hash BLOB PRIMARY KEY NOT NULL CHECK (length(token_hash) = 32),
    family_id TEXT NOT NULL REFERENCES auth_session_families(id) ON DELETE CASCADE,
    parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
) STRICT;

CREATE INDEX auth_refresh_tokens_family_idx ON auth_refresh_tokens(family_id);
CREATE INDEX auth_access_tokens_family_idx ON auth_access_tokens(family_id);
CREATE INDEX auth_access_tokens_account_idx ON auth_access_tokens(parent_account_id);
