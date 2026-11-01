CREATE TABLE parent_wakeup_registrations (
 family_id TEXT PRIMARY KEY NOT NULL REFERENCES auth_session_families(id) ON DELETE CASCADE,
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
 token_ciphertext BLOB NOT NULL CHECK(length(token_ciphertext) BETWEEN 29 AND 4124),
 registration_version INTEGER NOT NULL CHECK(registration_version > 0),
 updated_at TEXT NOT NULL
) STRICT;

CREATE INDEX parent_wakeup_registrations_account_idx ON parent_wakeup_registrations(parent_account_id);

CREATE TABLE parent_wakeup_registration_versions (
 family_id TEXT PRIMARY KEY NOT NULL REFERENCES auth_session_families(id) ON DELETE CASCADE,
 registration_version INTEGER NOT NULL CHECK(registration_version > 0)
) STRICT;

CREATE TABLE parent_call_events (
 event_id TEXT PRIMARY KEY NOT NULL,
 device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id) ON DELETE CASCADE,
 call_id TEXT NOT NULL,
 snapshot_sequence INTEGER NOT NULL CHECK(snapshot_sequence > 0),
 observed_at TEXT NOT NULL,
 created_at_ns INTEGER NOT NULL,
 expires_at_ns INTEGER NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('active','expired')),
 UNIQUE(device_id,call_id)
) STRICT;

CREATE INDEX parent_call_events_due_idx ON parent_call_events(status,expires_at_ns);

CREATE TABLE parent_call_event_deliveries (
 delivery_id TEXT PRIMARY KEY NOT NULL,
 event_id TEXT NOT NULL REFERENCES parent_call_events(event_id) ON DELETE CASCADE,
 family_id TEXT NOT NULL REFERENCES auth_session_families(id) ON DELETE CASCADE,
 status TEXT NOT NULL CHECK(status IN ('pending','leased','sent','expired','cancelled','failed')),
 attempts INTEGER NOT NULL CHECK(attempts BETWEEN 0 AND 5),
 next_attempt_at_ns INTEGER NOT NULL,
 lease_id TEXT,
 lease_expires_at_ns INTEGER,
 registration_version INTEGER NOT NULL CHECK(registration_version > 0),
 created_at_ns INTEGER NOT NULL,
 UNIQUE(event_id,family_id)
) STRICT;

CREATE INDEX parent_call_event_deliveries_due_idx ON parent_call_event_deliveries(status,next_attempt_at_ns,lease_expires_at_ns);

CREATE TRIGGER revoke_parent_push_registration AFTER UPDATE OF revoked_at ON auth_session_families
WHEN NEW.revoked_at IS NOT NULL BEGIN
 DELETE FROM parent_wakeup_registrations WHERE family_id=NEW.id;
 UPDATE parent_call_event_deliveries
 SET status='cancelled',lease_id=NULL,lease_expires_at_ns=NULL
 WHERE family_id=NEW.id AND status IN ('pending','leased');
END;

CREATE TRIGGER revoke_parent_push_device_events AFTER UPDATE OF state,parent_account_id ON devices
WHEN NEW.state!='active' OR NEW.parent_account_id!=OLD.parent_account_id BEGIN
 UPDATE parent_call_events SET status='expired' WHERE device_id=NEW.id AND status='active';
 UPDATE parent_call_event_deliveries
 SET status='expired',lease_id=NULL,lease_expires_at_ns=NULL
 WHERE event_id IN (SELECT event_id FROM parent_call_events WHERE device_id=NEW.id)
 AND status IN ('pending','leased');
END;
