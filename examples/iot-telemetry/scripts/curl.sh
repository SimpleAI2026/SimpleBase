#!/usr/bin/env bash
set -euo pipefail
: "${SIMPLEBASE_URL:?source .env}"
: "${SIMPLEBASE_PROJECT_ID:?source .env}"
: "${SIMPLEBASE_DATABASE_ID:?source .env}"
: "${SIMPLEBASE_API_KEY:?source .env}"
curl -fsS "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/databases/$SIMPLEBASE_DATABASE_ID/query" \
  -H "Authorization: Bearer $SIMPLEBASE_API_KEY" -H 'Content-Type: application/json' \
  -d '{"sql":"SELECT id, name, model FROM devices ORDER BY id LIMIT 10"}'
printf '\n'
curl -fsS "$SIMPLEBASE_URL/v1/projects/$SIMPLEBASE_PROJECT_ID/kv" \
  -H "Authorization: Bearer $SIMPLEBASE_API_KEY" -H 'Content-Type: application/json' \
  -d '{"type":"cmd","argvs":["SCAN","0","MATCH","online:*"]}'
printf '\n'
