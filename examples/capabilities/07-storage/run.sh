#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

KEY="examples/${RUN_ID}/note.txt"
NOTE="$(mktemp)"
printf 'hello-storage\n' >"$NOTE"
trap 'rm -f "$NOTE"' EXIT

step "upload" object upload --key "$KEY" --file "$NOTE"
expect '.data.key == $k' --arg k "$KEY"

step "list" object list --prefix "examples/${RUN_ID}/"
expect '[.data[]? | select(.key == $k)] | length == 1' --arg k "$KEY"

step "presign" object presign --key "$KEY"
expect '.data.url | type == "string" and length > 0'

step "delete" object delete --key "$KEY"
done_step

step "list-after" object list --prefix "examples/${RUN_ID}/"
expect '[.data[]? | select(.key == $k)] | length == 0' --arg k "$KEY"

finish_case
