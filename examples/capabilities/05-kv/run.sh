#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

COLOR="ex:${RUN_ID}:color"
VISITS="ex:${RUN_ID}:visits"
USERK="ex:${RUN_ID}:user"

step "set" kv exec --command SET --arg "$COLOR" --arg blue
expect '.data == "OK"'

step "get" kv exec --command GET --arg "$COLOR"
expect '.data == "blue"'

step "incr" kv exec --command INCR --arg "$VISITS"
expect '.data == 1'

step "hset" kv exec --command HSET --arg "$USERK" --arg name --arg alice
expect '.data == 1'

step "hget" kv exec --command HGET --arg "$USERK" --arg name
expect '.data == "alice"'

step "del" kv exec --command DEL --arg "$COLOR"
expect '.data == 1'

step "get-missing" kv exec --command GET --arg "$COLOR"
expect '.data == null'

finish_case
