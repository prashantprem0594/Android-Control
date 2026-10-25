ALTER TABLE commands ADD COLUMN request_key TEXT;
ALTER TABLE commands ADD COLUMN ttl_seconds INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX commands_request_key_idx ON commands(parent_account_id,device_id,request_key)
WHERE request_key IS NOT NULL;
