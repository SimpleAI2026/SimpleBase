#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/../lib/common.sh"

NAME="greet${RUN_ID}"

step "create" function create --name "$NAME" --file "$HERE/greet_v1.go" --description "capability example"
expect '.data.name == $n and (.data.active_version == 1 or .data.ActiveVersion == 1)' --arg n "$NAME"

step "get" function get --name "$NAME"
expect '.data.name == $n' --arg n "$NAME"

step "invoke-v1" function invoke --name "$NAME" --export Greet --input '{"name":"simplebase"}'
expect '.data.message == "hello simplebase"'

step "version-create" function versions create --name "$NAME" --file "$HERE/greet_v2.go" --note "v2"
expect '(.data.version == 2) or (.data.Version == 2) or (.data.latest_version == 2)'

step "test-v2" function test --name "$NAME" --version 2 --export Greet --input '{"name":"simplebase"}'
expect '.data.message == "hello v2 simplebase"'

step "activate" function activate --name "$NAME" --version 2
done_step

step "invoke-v2" function invoke --name "$NAME" --export Greet --input '{"name":"simplebase"}'
expect '.data.message == "hello v2 simplebase"'

finish_case
