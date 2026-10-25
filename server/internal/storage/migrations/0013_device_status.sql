CREATE TABLE device_status (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id),
 parent_account_id TEXT NOT NULL REFERENCES parent_accounts(id),
 received_at TEXT NOT NULL,
 android_api INTEGER NOT NULL CHECK(android_api BETWEEN 33 AND 100),
 device_owner INTEGER NOT NULL CHECK(device_owner IN (0,1)),
 dialer_role INTEGER NOT NULL CHECK(dialer_role IN (0,1)),
 vpn_consent INTEGER NOT NULL CHECK(vpn_consent IN (0,1)),
 exact_alarms INTEGER NOT NULL CHECK(exact_alarms IN (0,1))
) STRICT;
