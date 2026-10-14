#!/bin/bash
# usage: check.sh <impl.go>
d=$(mktemp -d); here=$(cd "$(dirname "$0")" && pwd)
cp "$1" "$d/cfg.go"; cp "$here/main_test.go" "$here/go.mod" "$d/"
cd "$d" && go test ./... >out.txt 2>&1 && echo PASS || { echo FAIL; cat out.txt; exit 1; }
