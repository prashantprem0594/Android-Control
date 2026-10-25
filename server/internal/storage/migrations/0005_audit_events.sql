-- No foreign keys: history must survive removal of the account or device.
CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    occurred_at TEXT NOT NULL,
    parent_account_id TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK(actor_type IN ('parent','device','anonymous','system')),
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN (
        'parent_registered','parent_login','parent_login_failed','parent_refresh',
        'parent_session_revoked','pairing_code_created','enrollment_started',
        'enrollment_completed','enrollment_proof_failed','device_revoked','device_authenticated','device_auth_failed')),
    target_id TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '' CHECK(reason IN ('','logout','refresh_reuse','parent_revoked','proof_failures'))
) STRICT;
CREATE INDEX audit_events_parent_idx ON audit_events(parent_account_id,id);

CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events BEGIN
    SELECT RAISE(ABORT,'audit events are append only');
END;
CREATE TRIGGER audit_events_no_delete BEFORE DELETE ON audit_events BEGIN
    SELECT RAISE(ABORT,'audit events are append only');
END;
-- REPLACE can otherwise bypass DELETE triggers when recursive_triggers is off.
CREATE TRIGGER audit_events_no_replace BEFORE INSERT ON audit_events
WHEN EXISTS(SELECT 1 FROM audit_events WHERE id=NEW.id) BEGIN
    SELECT RAISE(ABORT,'audit events are append only');
END;

CREATE TRIGGER audit_parent_registered AFTER INSERT ON parent_accounts BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    VALUES(NEW.created_at,NEW.id,'parent',NEW.id,'parent_registered',NEW.id);
END;
CREATE TRIGGER audit_parent_login AFTER INSERT ON auth_session_families BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    VALUES(NEW.created_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'parent_login',NEW.id);
END;
CREATE TRIGGER audit_parent_refresh AFTER UPDATE OF consumed_at ON auth_refresh_tokens
WHEN OLD.consumed_at IS NULL AND NEW.consumed_at IS NOT NULL BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    SELECT NEW.consumed_at,parent_account_id,'parent',parent_account_id,'parent_refresh',id
    FROM auth_session_families WHERE id=NEW.family_id;
END;
CREATE TRIGGER audit_parent_session_revoked AFTER UPDATE OF revoked_at ON auth_session_families
WHEN OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id,reason)
    VALUES(NEW.revoked_at,NEW.parent_account_id,'system','','parent_session_revoked',NEW.id,NEW.revocation_reason);
END;
CREATE TRIGGER audit_login_failure_insert AFTER INSERT ON auth_login_failures BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    VALUES(NEW.updated_at,COALESCE((SELECT id FROM parent_accounts WHERE email=NEW.email),''),
        'anonymous','','parent_login_failed','');
END;
CREATE TRIGGER audit_login_failure_update AFTER UPDATE ON auth_login_failures BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    VALUES(NEW.updated_at,COALESCE((SELECT id FROM parent_accounts WHERE email=NEW.email),''),
        'anonymous','','parent_login_failed','');
END;
CREATE TRIGGER audit_pairing_code AFTER INSERT ON pairing_codes BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    VALUES(NEW.created_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'pairing_code_created',NEW.parent_account_id);
END;
CREATE TRIGGER audit_enrollment_started AFTER INSERT ON devices BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    VALUES(NEW.created_at,NEW.parent_account_id,'system','','enrollment_started',NEW.id);
END;
CREATE TRIGGER audit_enrollment_completed AFTER UPDATE OF state ON devices
WHEN OLD.state='pending' AND NEW.state='active' BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    VALUES(NEW.enrolled_at,NEW.parent_account_id,'device',NEW.id,'enrollment_completed',NEW.id);
END;
CREATE TRIGGER audit_device_revoked AFTER UPDATE OF state ON devices
WHEN OLD.state!='revoked' AND NEW.state='revoked' BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id,reason)
    VALUES(NEW.revoked_at,NEW.parent_account_id,
        CASE WHEN NEW.revocation_reason='parent_revoked' THEN 'parent' ELSE 'system' END,
        CASE WHEN NEW.revocation_reason='parent_revoked' THEN NEW.parent_account_id ELSE '' END,
        'device_revoked',NEW.id,NEW.revocation_reason);
END;
CREATE TRIGGER audit_enrollment_proof_failed AFTER UPDATE OF verification_attempts ON device_enrollments
WHEN NEW.verification_attempts>OLD.verification_attempts BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    SELECT strftime('%Y-%m-%dT%H:%M:%fZ','now'),parent_account_id,'anonymous','','enrollment_proof_failed',id
    FROM devices WHERE id=NEW.device_id;
END;
ALTER TABLE device_sessions ADD COLUMN issued_at TEXT NOT NULL DEFAULT '';
CREATE TRIGGER audit_device_authenticated AFTER INSERT ON device_sessions BEGIN
    INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
    SELECT NEW.issued_at,parent_account_id,'device',id,'device_authenticated',id
    FROM devices WHERE id=NEW.device_id;
END;
