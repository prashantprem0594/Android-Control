CREATE TABLE remote_assistance_sessions (
    id TEXT PRIMARY KEY NOT NULL,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('active','closed','expired')),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    closed_at TEXT,
    offer_sdp TEXT NOT NULL DEFAULT '',
    answer_sdp TEXT NOT NULL DEFAULT '',
    ice_json TEXT NOT NULL DEFAULT '[]'
) STRICT;
CREATE UNIQUE INDEX remote_assistance_one_active_device
    ON remote_assistance_sessions(device_id) WHERE status='active';
CREATE INDEX remote_assistance_parent_idx ON remote_assistance_sessions(parent_account_id,created_at);
