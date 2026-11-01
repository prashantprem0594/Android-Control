-- SQLite cannot modify the Android API CHECK constraint in place. Rebuild the
-- table so existing installations can accept the child app's minSdk (28), then
-- retain their most recent status while adding connectivity fields.
ALTER TABLE device_status RENAME TO device_status_legacy;

CREATE TABLE device_status (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id),
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 received_at TEXT NOT NULL,
 android_api INTEGER NOT NULL CHECK(android_api BETWEEN 21 AND 100),
 device_owner INTEGER NOT NULL CHECK(device_owner IN (0,1)),
 dialer_role INTEGER NOT NULL CHECK(dialer_role IN (0,1)),
 vpn_consent INTEGER NOT NULL CHECK(vpn_consent IN (0,1)),
 exact_alarms INTEGER NOT NULL CHECK(exact_alarms IN (0,1)),
 network_connected INTEGER NOT NULL DEFAULT 0 CHECK(network_connected IN (0,1)),
 wifi_connected INTEGER NOT NULL DEFAULT 0 CHECK(wifi_connected IN (0,1)),
 cellular_connected INTEGER NOT NULL DEFAULT 0 CHECK(cellular_connected IN (0,1)),
 vpn_active INTEGER NOT NULL DEFAULT 0 CHECK(vpn_active IN (0,1))
) STRICT;

INSERT INTO device_status(device_id,parent_account_id,received_at,android_api,device_owner,dialer_role,vpn_consent,exact_alarms)
SELECT device_id,parent_account_id,received_at,android_api,device_owner,dialer_role,vpn_consent,exact_alarms
FROM device_status_legacy;

DROP TABLE device_status_legacy;
