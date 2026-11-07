#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

# 文档进阶：patch（merge 语义）、batch、export → import、collection schema 校验、
# filter/fields 查询、truncate。

SUF="$(python3 -c 'import secrets; print(secrets.token_hex(3))')"
SRC="notes_${SUF}"
DST="imported_${SUF}"
STRICT="strict_${SUF}"
TMPDIR_CASE="$(mktemp -d "${TMPDIR:-/tmp}/sb-docadv.XXXXXX")"
EXPORT_FILE="$TMPDIR_CASE/export.ndjson"
DB=""
CREATED_DB=0
DOC=""

cleanup() {
  if [[ "$CREATED_DB" == "1" && -n "$DB" ]]; then
    SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
      SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
      "$SIMPLEBASE_BIN" database delete --id "$DB" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMPDIR_CASE"
}
trap cleanup EXIT

sb_json() {
  SIMPLEBASE_URL="$SIMPLEBASE_URL" SIMPLEBASE_TOKEN="$SIMPLEBASE_TOKEN" \
    SIMPLEBASE_PROJECT_ID="$SIMPLEBASE_PROJECT_ID" SIMPLEBASE_OUTPUT=json \
    "$SIMPLEBASE_BIN" "$@" 2>&1 || true
}

# ── 1. 数据库与集合 ───────────────────────────────────────
DB="$(sb_json database list | jq -r '.data.databases[0].id // empty')"
if [[ -n "$DB" ]]; then
  STEP_NAME="reuse-db"
  STEP_CMD="simplebase database list"
  STEP_MS=0
  record 1 "复用已有数据库 ${DB}（项目单文件限制：每个项目只有一个用户库）"
else
  step "create-db" database create --name "cap12-${SUF}" --data-model collection
  expect '.data.id != "" and .data.data_model == "collection"'
  DB="$(jq_out '.data.id')"
  CREATED_DB=1
fi
if [[ -z "$DB" || "$DB" == "null" ]]; then
  echo "FAIL ${CASE}/no-database"
  exit 1
fi

step "create-collection" collection create --database "$DB" --name "$SRC"
done_step

# ── 2. patch：只覆盖提交的字段 ───────────────────────────
step "insert" document insert --database "$DB" --collection "$SRC" \
  --json '{"title":"hello","body":"world","n":1}'
expect '.data.id != "" and .data.title == "hello" and .data.body == "world"'
DOC="$(jq_out '.data.id')"

step "patch" document patch --database "$DB" --collection "$SRC" --id "$DOC" \
  --json '{"body":"patched"}'
expect '.data.id == $id and .data.title == "hello" and .data.body == "patched" and .data.n == 1' --arg id "$DOC"

# ── 3. batch：一次写多篇 ─────────────────────────────────
step "batch" document batch --database "$DB" --collection "$SRC" \
  --json '[{"title":"b1","n":2},{"title":"b2","n":3}]'
expect '(.data.items | length) == 2 and (.data.items | map(.data.title) | sort == ["b1","b2"])'

step "list" document list --database "$DB" --collection "$SRC"
expect '(.data.rows | length) == 3'

# ── 4. collection schema-put + filter/fields ─────────────
step "schema-put" collection schema-put --database "$DB" --collection "$SRC" \
  --json '{"fields":[{"name":"title","type":"text","required":true},{"name":"body","type":"text"},{"name":"n","type":"number"}]}'
expect '.data.name == $c and (.data.fields | length) == 3' --arg c "$SRC"

step "schema-get" collection schema-get --database "$DB" --collection "$SRC"
expect '.data.fields | map(select(.name == "title" and .required == true)) | length == 1'

step "filter-fields" document list --database "$DB" --collection "$SRC" \
  --filter 'n >= 2' --fields title --count 1
expect '(.data.rows | length) == 2 and (.data.rows | all(has("title") and (has("body") | not))) and .data.total == 2'

# ── 5. export → import 到新集合 ──────────────────────────
step "export" document export --db "$DB" --collection "$SRC" --format ndjson --file "$EXPORT_FILE"
expect '.data.bytes > 0 and .data.file != ""'
EXPORTED="$(wc -l <"$EXPORT_FILE" | tr -d ' ')"
STEP_NAME="export-rows"
STEP_CMD="wc -l $EXPORT_FILE"
STEP_MS=0
STEP_OK=0
STEP_WHY=""
if [[ "$EXPORTED" == "3" ]]; then
  STEP_OK=1
else
  STEP_WHY="导出 $EXPORTED 行，期望 3 行"
fi
record "$STEP_OK" "$STEP_WHY"

step "create-import-target" collection create --database "$DB" --name "$DST"
done_step

step "import" document import --db "$DB" --collection "$DST" --format ndjson --file "$EXPORT_FILE"
expect '.data.scanned == 3 and .data.imported == 3 and .data.dry_run == false'

step "list-imported" document list --database "$DB" --collection "$DST"
expect '(.data.rows | length) == 3 and (.data.rows | map(.title) | sort == ["b1","b2","hello"])'

# ── 6. schema 校验：缺 required 字段被拒绝 ────────────────
step "strict-schema-put" collection schema-put --database "$DB" --collection "$STRICT" \
  --json '{"fields":[{"name":"title","type":"text","required":true},{"name":"qty","type":"number"}]}'
expect '(.data.fields | map(select(.name == "title" and .required == true)) | length) == 1'

step "strict-insert-invalid" document insert --database "$DB" --collection "$STRICT" --json '{"qty":1}'
STEP_NAME="strict-insert-invalid"
STEP_CMD="simplebase document insert ... --json '{\"qty\":1}'"
STEP_MS=0
STEP_WHY=""
if [[ "$STEP_OK" -eq 1 ]]; then
  STEP_OK=0
  STEP_WHY="schema 要求的 title 缺失却插入成功"
elif printf '%s' "$SB_ERR" | jq -e '.error.code == "required_field"' >/dev/null 2>&1; then
  STEP_OK=1
else
  STEP_OK=0
  STEP_WHY="期望 required_field，实际：$(printf '%s%s' "$SB_OUT" "$SB_ERR" | redact)"
fi
record "$STEP_OK" "$STEP_WHY"

step "strict-insert-valid" document insert --database "$DB" --collection "$STRICT" \
  --json '{"title":"checked","qty":2}'
expect '.data.title == "checked" and .data.qty == 2'

# ── 7. truncate ──────────────────────────────────────────
step "truncate" collection truncate --database "$DB" --collection "$DST"
done_step
step "list-truncated" document list --database "$DB" --collection "$DST"
expect '(.data.rows | length) == 0'

# ── 8. 清理 ───────────────────────────────────────────────
if [[ "$CREATED_DB" == "1" ]]; then
  step "delete-db" database delete --id "$DB"
  done_step
  CREATED_DB=0
fi

finish_case
