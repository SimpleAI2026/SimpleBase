INSERT INTO tickets SELECT 'ticket-demo', '检查昨日异常请求', 'high', 'open', '2026-10-03T12:00:00Z', 1 WHERE NOT EXISTS (SELECT 1 FROM tickets WHERE id = 'ticket-demo');
INSERT INTO ticket_events SELECT 'event-demo', 'ticket-demo', 'created', '2026-10-02T12:00:00Z' WHERE NOT EXISTS (SELECT 1 FROM ticket_events WHERE id = 'event-demo');
