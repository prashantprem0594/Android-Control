CREATE TABLE app_metadata (
    key TEXT PRIMARY KEY NOT NULL,
    value BLOB NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
