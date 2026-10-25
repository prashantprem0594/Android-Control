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
        'device_authenticated','device_auth_failed','database_restored','command_created','policy_updated','command_delivered','command_succeeded','command_failed','command_expired','policy_applied','policy_partial','policy_failed')),
    target_id TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '' CHECK(reason IN ('','logout','refresh_reuse','parent_revoked','proof_failures','device_owner_required','protected_package','package_unavailable','package_change_failed','internet_pause_unavailable','interrupted'))
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



CREATE TABLE device_policy_reports (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id),
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 version INTEGER NOT NULL CHECK(version>0),
 result TEXT NOT NULL CHECK(result IN ('applied','partial','failed')),
 reason TEXT NOT NULL CHECK(
  (result='applied' AND reason='') OR
  (result IN ('partial','failed') AND reason IN ('device_owner_required','protected_package','package_unavailable','package_change_failed','internet_pause_unavailable','interrupted'))
 ),
 reported_at TEXT NOT NULL
) STRICT;
CREATE TRIGGER audit_policy_report_insert AFTER INSERT ON device_policy_reports BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id,reason)
 VALUES(NEW.reported_at,NEW.parent_account_id,'device',NEW.device_id,'policy_'||NEW.result,NEW.device_id,NEW.reason);
END;
CREATE TRIGGER audit_policy_report_update AFTER UPDATE ON device_policy_reports
WHEN NEW.version!=OLD.version OR NEW.result!=OLD.result OR NEW.reason!=OLD.reason BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id,reason)
 VALUES(NEW.reported_at,NEW.parent_account_id,'device',NEW.device_id,'policy_'||NEW.result,NEW.device_id,NEW.reason);
END;

