-- Extend the action vocabulary without changing an already-applied migration.
CREATE TABLE audit_events_copy AS SELECT * FROM audit_events;
DROP TRIGGER audit_events_no_delete;
DROP TABLE audit_events;
CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    occurred_at TEXT NOT NULL,
    parent_account_id TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK(actor_type IN ('parent','device','anonymous','system')),
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN (
        'parent_registered','parent_login','parent_login_failed','parent_refresh',
        'parent_session_revoked','pairing_code_created','enrollment_started',
        'enrollment_completed','enrollment_proof_failed','device_revoked',
        'device_authenticated','device_auth_failed','database_restored','command_created','policy_updated')),
    target_id TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '' CHECK(reason IN ('','logout','refresh_reuse','parent_revoked','proof_failures'))
) STRICT;
INSERT INTO audit_events SELECT * FROM audit_events_copy ORDER BY id;
DROP TABLE audit_events_copy;
CREATE INDEX audit_events_parent_idx ON audit_events(parent_account_id,id);
CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events BEGIN
    SELECT RAISE(ABORT,'audit events are append only');
END;
CREATE TRIGGER audit_events_no_delete BEFORE DELETE ON audit_events BEGIN
    SELECT RAISE(ABORT,'audit events are append only');
END;
CREATE TRIGGER audit_events_no_replace BEFORE INSERT ON audit_events
WHEN EXISTS(SELECT 1 FROM audit_events WHERE id=NEW.id) BEGIN
    SELECT RAISE(ABORT,'audit events are append only');
END;


CREATE TABLE commands (
 id TEXT PRIMARY KEY NOT NULL,
 device_id TEXT NOT NULL REFERENCES devices(id),
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 type TEXT NOT NULL CHECK(type='LOCK_DEVICE'),
 payload TEXT NOT NULL CHECK(payload='{}'),
 issued_at TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','delivered','succeeded','failed','expired'))
) STRICT;
CREATE INDEX commands_device_idx ON commands(device_id,issued_at,id);
CREATE TABLE device_policies (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id),
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 version INTEGER NOT NULL CHECK(version>0),
 internet_paused INTEGER NOT NULL CHECK(internet_paused IN (0,1)),
 blocked_packages TEXT NOT NULL,
 updated_at TEXT NOT NULL
) STRICT;
CREATE TRIGGER audit_command_created AFTER INSERT ON commands BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.issued_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'command_created',NEW.id);
END;
CREATE TRIGGER audit_policy_created AFTER INSERT ON device_policies BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.updated_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'policy_updated',NEW.device_id);
END;
CREATE TRIGGER audit_policy_updated AFTER UPDATE ON device_policies BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.updated_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'policy_updated',NEW.device_id);
END;

