#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

NAME="sql-${RUN_ID}"
step "create-db" database create --name "$NAME" --data-model sql
expect '.data.id != "" and .data.name == $n and .data.data_model == "sql"' --arg n "$NAME"
DB="$(jq_out '.data.id')"

if [[ -z "$DB" || "$DB" == "null" ]]; then
  echo "FAIL ${CASE}/no-database-id"
  exit 1
fi

step "create-table" schema create-table --database "$DB" --name items \
  --columns '[{"name":"id","type":"BIGINT","nullable":false},{"name":"title","type":"VARCHAR"}]'
done_step

step "add-column" schema add-column --database "$DB" --table items \
  --column '{"name":"qty","type":"INTEGER"}'
done_step

step "schema-show" schema show --database "$DB"
expect '.data.tables | map(select(.name == "items")) | .[0].columns | map(.name) | index("id") != null and index("title") != null and index("qty") != null'

step "insert" sql exec --database "$DB" --statement "INSERT INTO items (id, title, qty) VALUES (1, 'demo', 3)"
done_step

step "query-insert" sql query --database "$DB" --statement "SELECT id, title, qty FROM items" --max-rows 10
expect '.data.row_count == 1 and (.data.rows[0][0] == 1) and (.data.rows[0][1] == "demo") and ((.data.rows[0][2] == 3) or (.data.rows[0][2] == "3"))'

step "update" sql exec --database "$DB" --statement "UPDATE items SET title = 'edited' WHERE id = 1"
done_step

step "query-update" sql query --database "$DB" --statement "SELECT title FROM items WHERE id = 1" --max-rows 10
expect '.data.rows[0][0] == "edited"'

step "delete-row" sql exec --database "$DB" --statement "DELETE FROM items WHERE id = 1"
done_step

step "query-empty" sql query --database "$DB" --statement "SELECT id FROM items" --max-rows 10
expect '.data.row_count == 0'

step "schema-rows" schema rows --database "$DB" --table items --limit 10
done_step

step "delete-db" database delete --id "$DB"
done_step

finish_case
