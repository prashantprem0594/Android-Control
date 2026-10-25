CREATE TABLE remote_call_snapshots (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
 observed_at TEXT NOT NULL,
 calls_json TEXT NOT NULL
) STRICT;
