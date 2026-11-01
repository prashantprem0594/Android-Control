CREATE TABLE parent_call_event_fanout_cursors (
 event_id TEXT PRIMARY KEY NOT NULL REFERENCES parent_call_events(event_id) ON DELETE CASCADE,
 last_family_id TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE parent_push_processing_cursors (
 name TEXT PRIMARY KEY NOT NULL,
 last_event_id TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX parent_call_events_active_cursor_idx ON parent_call_events(status,event_id);
CREATE INDEX parent_wakeup_registrations_account_family_idx
 ON parent_wakeup_registrations(parent_account_id,family_id);
