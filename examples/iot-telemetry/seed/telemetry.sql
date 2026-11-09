INSERT INTO devices SELECT 'device-01', '温室一号', 'sensor-v1' WHERE NOT EXISTS (SELECT 1 FROM devices WHERE id = 'device-01');
INSERT INTO devices SELECT 'device-02', '温室二号', 'sensor-v1' WHERE NOT EXISTS (SELECT 1 FROM devices WHERE id = 'device-02');
INSERT INTO readings SELECT 'device-01', '2026-10-02T12:00:00Z', 'temperature', 22.5 WHERE NOT EXISTS (SELECT 1 FROM readings WHERE device_id = 'device-01' AND ts = '2026-10-02T12:00:00Z');
