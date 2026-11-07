#!/usr/bin/env bash
# smoke/community.sh — 社区 BFF 业务冒烟（planv6.4 §4.4）
#
# 断言：POST /api/posts 成功；含 spam 被拒；重复点赞只计一次；DELETE 后附件对象一并删除；
#       无 provider 时 summary 返回 501。
#
# 前置：SIMPLEBASE_URL / SIMPLEBASE_API_KEY / SIMPLEBASE_PROJECT_ID /
#       SIMPLEBASE_DATABASE_ID_COMMUNITY（多库站点，没有不带后缀的 DATABASE_ID）
# 用法：SMOKE_STEP=S8 SMOKE_ENV=A SMOKE_BFF_URL=http://127.0.0.1:28106 bash smoke/community.sh

set -uo pipefail
# shellcheck source=../lib.sh
. "$(cd "$(dirname "$0")" && pwd)/../lib.sh"
SV_SUITE="${SV_SUITE:-smoke}"

: "${SIMPLEBASE_URL:?需要 SIMPLEBASE_URL（先 source 运行时 env）}"
: "${SIMPLEBASE_API_KEY:?需要 SIMPLEBASE_API_KEY}"
: "${SIMPLEBASE_PROJECT_ID:?需要 SIMPLEBASE_PROJECT_ID}"
CDB="${SIMPLEBASE_DATABASE_ID_COMMUNITY:-${SIMPLEBASE_DATABASE_ID:-}}"
: "${CDB:?需要 SIMPLEBASE_DATABASE_ID_COMMUNITY（community 库）}"

BFF="${1:-${SMOKE_BFF_URL:-}}"
if [ -z "$BFF" ]; then
  echo "smoke/community.sh: 需要 BFF URL（参数或 SMOKE_BFF_URL）" >&2
  exit 1
fi
BFF="${BFF%/}"
API_BASE="${SIMPLEBASE_URL%/}/v1/projects/$SIMPLEBASE_PROJECT_ID"

smoke_start
smoke_init_tmp
SMOKE_STARTED="$(now_ms)"

# ── 1. BFF 健康 ─────────────────────────────────────────
code="$(sb_request GET "$BFF/health")"
[ "$code" = "200" ]
smoke_check "bff-health" $? "code=$code"

# ── 2. POST /api/posts 成功 ─────────────────────────────
code="$(sb_request POST "$BFF/api/posts" '{"title":"verify 冒烟","body":"来自 examples-verify harness"}')"
post_id="$(sb_body | jq -r '.id // .document.id // empty' 2>/dev/null)"
[ "$code" = "201" ] && [ -n "$post_id" ]
smoke_check "create-post" $? "code=$code id=${post_id:0:8}"

# ── 3. spam 被拒 ────────────────────────────────────────
code="$(sb_request POST "$BFF/api/posts" '{"title":"spam","body":"spam spam"}')"
[ "$code" = "400" ]
smoke_check "reject-spam" $? "code=$code"

# ── 4. 重复点赞只计一次 ─────────────────────────────────
code1="$(sb_request POST "$BFF/api/posts/$post_id/likes" '{}')"
code2="$(sb_request POST "$BFF/api/posts/$post_id/likes" '{}')"
likes="$(sql_scalar "$CDB" "SELECT COUNT(*) FROM likes WHERE post_id = '$post_id'")"
[ "$code1" = "201" ] && [ "$code2" = "200" ] && [ "$likes" = "1" ]
smoke_check "duplicate-like-once" $? "first=$code1 second=$code2 likes=$likes"

# ── 5. summary：无 provider → 501（有 provider 时 200/502 也接受，仅记录）──
code="$(sb_request POST "$BFF/api/posts/$post_id/summary" '{}')"
summary_note=""
case "$code" in
  501) ;;
  200|502) summary_note="provider 已配置（summary=${code}），501 无 provider 分支未验证" ;;
esac
[ "$code" = "501" ] || [ -n "$summary_note" ]
smoke_check "summary-no-provider-501" $? "code=$code"

# ── 6. 附件：DELETE 后对象一并删除 ──────────────────────
# 服务端 CreateDocument 生成 UUID、忽略 seed.id，所以种子帖子要按 title 找实际 id。
code="$(sb_request GET "$BFF/api/posts")"
seed_id="$(sb_body | jq -r '[.[] | select(.title == "欢迎" and ((.attachment_key // "") != ""))][0].id // empty' 2>/dev/null)"
seed_key="$(sb_body | jq -r '[.[] | select(.title == "欢迎" and ((.attachment_key // "") != ""))][0].attachment_key // empty' 2>/dev/null)"
[ "$code" = "200" ] && [ -n "$seed_id" ] && [ -n "$seed_key" ]
smoke_check "seed-post-with-attachment" $? "code=$code id=${seed_id:0:8} key=$seed_key"
if [ -z "$seed_id" ] || [ -z "$seed_key" ]; then
  smoke_finish "缺少带附件的种子帖子，附件删除用例无法验证"
  exit 1
fi

code="$(sb_request GET "$API_BASE/s3/objects")"
has_before="$(sb_body | jq --arg key "$seed_key" '[.[] | select(.key == $key)] | length' 2>/dev/null || echo 0)"
[ "$code" = "200" ] && [ "$has_before" = "1" ]
smoke_check "attachment-exists" $? "code=$code key=$seed_key"

code="$(sb_request DELETE "$BFF/api/posts/$seed_id")"
[ "$code" = "200" ]
smoke_check "delete-post" $? "code=$code"

code="$(sb_request GET "$API_BASE/s3/objects")"
has_after="$(sb_body | jq --arg key "$seed_key" '[.[] | select(.key == $key)] | length' 2>/dev/null || echo 1)"
[ "$code" = "200" ] && [ "$has_after" = "0" ]
smoke_check "attachment-deleted-with-post" $? "code=$code remaining=$has_after"

code="$(sb_request GET "$BFF/api/posts")"
still_there="$(sb_body | jq --arg id "$seed_id" '[.[] | select(.id == $id)] | length' 2>/dev/null || echo 1)"
[ "$code" = "200" ] && [ "$still_there" = "0" ]
smoke_check "document-deleted" $? "code=$code deleted_present=$still_there"

SMOKE_MS=$(( $(now_ms) - SMOKE_STARTED ))
extra="post ${post_id:0:8}，likes=1，附件删除"
[ -n "$summary_note" ] && extra="${extra}；$summary_note"
smoke_finish "$extra"
