#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

NAME="doc-${RUN_ID}"
step "create-db" database create --name "$NAME"
expect '.data.id != "" and .data.name == $n and (.data.data_model == "collection" or .data.data_model == null or .data.data_model == "")' --arg n "$NAME"
DB="$(jq_out '.data.id')"
if [[ -z "$DB" || "$DB" == "null" ]]; then
  echo "FAIL ${CASE}/no-database-id"
  exit 1
fi

step "create-collection" collection create --database "$DB" --name notes
done_step

step "insert" document insert --database "$DB" --collection notes --json '{"title":"hello","body":"world"}'
expect '.data.id != "" and .data.title == "hello"'
DOC="$(jq_out '.data.id')"

step "list" document list --database "$DB" --collection notes
expect '.data.rows | map(select(.id == $id and .title == "hello")) | length == 1' --arg id "$DOC"

step "update" document update --database "$DB" --collection notes --id "$DOC" --json '{"title":"edited","body":"world"}'
expect '.data.title == "edited"'

step "list-updated" document list --database "$DB" --collection notes
expect '.data.rows | map(select(.id == $id and .title == "edited")) | length == 1' --arg id "$DOC"

step "delete-doc" document delete --database "$DB" --collection notes --id "$DOC"
done_step

step "list-empty" document list --database "$DB" --collection notes
expect '[.data.rows[]? | select(.id == $id)] | length == 0' --arg id "$DOC"

step "delete-db" database delete --id "$DB"
done_step

finish_case
