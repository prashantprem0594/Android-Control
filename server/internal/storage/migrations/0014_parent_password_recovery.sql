-- Preserve all prior events and append-only protections while extending vocabulary.
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
  'device_authenticated','device_auth_failed','database_restored','command_created',
  'policy_updated','command_delivered','command_succeeded','command_failed',
  'command_expired','policy_applied','policy_partial','policy_failed','app_inventory_updated','parent_password_reset')),
 target_id TEXT NOT NULL,
 reason TEXT NOT NULL DEFAULT '' CHECK(reason IN ('','logout','refresh_reuse','parent_revoked','proof_failures',
  'device_owner_required','protected_package','package_unavailable','package_change_failed',
  'internet_pause_unavailable','interrupted','password_reset'))
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
CREATE TRIGGER audit_parent_password_reset AFTER UPDATE OF password_hash ON parent_accounts
WHEN OLD.password_hash!=NEW.password_hash BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.updated_at,NEW.id,'system','','parent_password_reset',NEW.id);
END;
