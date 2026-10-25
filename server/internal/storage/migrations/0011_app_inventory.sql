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
        'device_authenticated','device_auth_failed','database_restored','command_created','policy_updated','command_delivered','command_succeeded','command_failed','command_expired','policy_applied','policy_partial','policy_failed','app_inventory_updated')),
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




CREATE TABLE device_app_inventory (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id),
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 apps_json TEXT NOT NULL,
 updated_at TEXT NOT NULL
) STRICT;
CREATE TRIGGER audit_inventory_insert AFTER INSERT ON device_app_inventory BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.updated_at,NEW.parent_account_id,'device',NEW.device_id,'app_inventory_updated',NEW.device_id);
END;
CREATE TRIGGER audit_inventory_update AFTER UPDATE ON device_app_inventory WHEN NEW.apps_json!=OLD.apps_json BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.updated_at,NEW.parent_account_id,'device',NEW.device_id,'app_inventory_updated',NEW.device_id);
END;

