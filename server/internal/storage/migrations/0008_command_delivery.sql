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
        'device_authenticated','device_auth_failed','database_restored','command_created','policy_updated','command_delivered','command_succeeded','command_failed','command_expired')),
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


ALTER TABLE commands ADD COLUMN status_at TEXT NOT NULL DEFAULT '';
CREATE TRIGGER commands_status_transition BEFORE UPDATE OF status ON commands
WHEN OLD.status!=NEW.status AND NOT (
 (OLD.status='queued' AND NEW.status IN ('delivered','expired')) OR
 (OLD.status='delivered' AND NEW.status IN ('succeeded','failed','expired'))
) BEGIN SELECT RAISE(ABORT,'invalid command transition'); END;
CREATE TRIGGER audit_command_status AFTER UPDATE OF status ON commands
WHEN OLD.status!=NEW.status BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.status_at,NEW.parent_account_id,
 CASE WHEN NEW.status='expired' THEN 'system' ELSE 'device' END,
 CASE WHEN NEW.status='expired' THEN '' ELSE NEW.device_id END,
 'command_'||NEW.status,NEW.id);
END;

