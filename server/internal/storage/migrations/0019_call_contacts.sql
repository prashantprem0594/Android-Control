CREATE TABLE device_call_contacts (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 version INTEGER NOT NULL CHECK(version > 0),
 contacts_json TEXT NOT NULL,
 updated_at TEXT NOT NULL
) STRICT;

-- Contact shortcuts are parent-maintained policy. Audit identifiers only.
CREATE TRIGGER audit_call_contacts_created AFTER INSERT ON device_call_contacts BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.updated_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'policy_updated',NEW.device_id);
END;
CREATE TRIGGER audit_call_contacts_updated AFTER UPDATE ON device_call_contacts BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.updated_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'policy_updated',NEW.device_id);
END;
