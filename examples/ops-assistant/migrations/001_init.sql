CREATE TABLE IF NOT EXISTS tickets (id VARCHAR, title VARCHAR, priority VARCHAR, status VARCHAR, due_at VARCHAR, version BIGINT);
CREATE TABLE IF NOT EXISTS ticket_events (id VARCHAR, ticket_id VARCHAR, kind VARCHAR, created_at VARCHAR);
CREATE TABLE IF NOT EXISTS sla_marks (ticket_id VARCHAR, level VARCHAR, run_id VARCHAR);
