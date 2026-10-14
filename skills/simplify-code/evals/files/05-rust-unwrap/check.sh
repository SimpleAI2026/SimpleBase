#!/bin/bash
d=$(mktemp -d); here=$(cd "$(dirname "$0")" && pwd); cp "$1" $d/impl.rs
IMPL=$d/impl.rs rustc --edition 2021 -A warnings -o $d/t "$here/test.rs" 2>$d/err && $d/t 2>/dev/null || { echo FAIL; cat $d/err | head -20; exit 1; }
