CREATE TABLE wakeup_registration_tokens (
 device_id TEXT PRIMARY KEY NOT NULL REFERENCES devices(id),
 registration_ciphertext BLOB NOT NULL CHECK(length(registration_ciphertext) BETWEEN 29 AND 4124),
 updated_at TEXT NOT NULL
) STRICT;
CREATE TRIGGER delete_revoked_wakeup_registration AFTER UPDATE OF state ON devices
WHEN NEW.state!='active' BEGIN
 DELETE FROM wakeup_registration_tokens WHERE device_id=NEW.id;
END;
