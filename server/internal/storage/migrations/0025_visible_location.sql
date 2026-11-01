-- A location is only recorded while the child has explicitly started its
-- visible foreground sharing session.  Coordinates use E7 integers so the
-- database does not silently lose precision through text conversion.
CREATE TABLE visible_location_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    latitude_e7 INTEGER NOT NULL CHECK(latitude_e7 BETWEEN -900000000 AND 900000000),
    longitude_e7 INTEGER NOT NULL CHECK(longitude_e7 BETWEEN -1800000000 AND 1800000000),
    accuracy_meters REAL NOT NULL CHECK(accuracy_meters >= 0 AND accuracy_meters <= 100000),
    captured_at TEXT NOT NULL,
    received_at TEXT NOT NULL
);

CREATE INDEX visible_location_samples_device_received
    ON visible_location_samples(device_id, received_at DESC);
