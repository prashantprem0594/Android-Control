ALTER TABLE remote_call_snapshots ADD COLUMN snapshot_sequence INTEGER NOT NULL DEFAULT 0;

CREATE TABLE remote_call_snapshot_counters (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
 sequence INTEGER NOT NULL CHECK(sequence > 0)
) STRICT;
