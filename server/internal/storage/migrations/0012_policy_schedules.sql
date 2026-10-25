ALTER TABLE device_policies ADD COLUMN always_allowed_packages TEXT NOT NULL DEFAULT '[]';
ALTER TABLE device_policies ADD COLUMN schedules TEXT NOT NULL DEFAULT '[]';
ALTER TABLE device_policy_reports ADD COLUMN state_key TEXT NOT NULL DEFAULT '';
DROP TRIGGER audit_policy_report_update;
CREATE TRIGGER audit_policy_report_update AFTER UPDATE ON device_policy_reports
WHEN NEW.version!=OLD.version OR NEW.result!=OLD.result OR NEW.reason!=OLD.reason OR NEW.state_key!=OLD.state_key BEGIN
 INSERT INTO audit_events(occurred_at,parent_account_id,actor_type,actor_id,action,target_id,reason)
 VALUES(NEW.reported_at,NEW.parent_account_id,'device',NEW.device_id,'policy_'||NEW.result,NEW.device_id,NEW.reason);
END;
