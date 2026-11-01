-- Extend the durable command vocabulary for bounded app and Settings requests.
DROP TRIGGER audit_command_created;
DROP TRIGGER commands_status_transition;
DROP TRIGGER audit_command_status;
DROP INDEX commands_device_idx;
DROP INDEX commands_request_key_idx;

ALTER TABLE commands RENAME TO commands_previous;

CREATE TABLE commands (
 id TEXT PRIMARY KEY NOT NULL,
 device_id TEXT NOT NULL REFERENCES devices(id),
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 type TEXT NOT NULL CHECK(type IN ('LOCK_DEVICE','ANSWER_CALL','REJECT_CALL','END_CALL','OPEN_APP','OPEN_SETTINGS')),
 payload TEXT NOT NULL,
 issued_at TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','delivered','succeeded','failed','expired')),
 request_key TEXT,
 ttl_seconds INTEGER NOT NULL,
 status_at TEXT NOT NULL DEFAULT ''
) STRICT;

INSERT INTO commands(id,device_id,parent_account_id,type,payload,issued_at,expires_at,status,request_key,ttl_seconds,status_at)
SELECT id,device_id,parent_account_id,type,payload,issued_at,expires_at,status,request_key,ttl_seconds,status_at
FROM commands_previous;

DROP TABLE commands_previous;

CREATE INDEX commands_device_idx ON commands(device_id,issued_at,id);
CREATE UNIQUE INDEX commands_request_key_idx ON commands(parent_account_id,device_id,request_key)
WHERE request_key IS NOT NULL;

CREATE TRIGGER audit_command_created AFTER INSERT ON commands BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id)
 VALUES(NEW.issued_at,NEW.parent_account_id,'parent',NEW.parent_account_id,'command_created',NEW.id);
END;
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
