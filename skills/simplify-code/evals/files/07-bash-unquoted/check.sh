#!/bin/bash
# usage: check.sh impl.sh  -- checks (a) original behavior on simple names, (b) reports space-handling separately
impl=$(readlink -f "$1"); t=$(mktemp -d); cd $t; fail=0
mkdir src; touch src/a.txt src/b.txt src/c.log
out=$(bash $impl src dst); [ "$out" = "copied 2 files" ] && [ -f dst/a.txt ] && [ -f dst/b.txt ] && [ ! -e dst/c.log ] || { echo "FAIL simple: $out"; fail=1; }
out=$(bash $impl 2>&1); rc=$?; [ $rc = 2 ] && [ "$out" = "usage: backup.sh SRC DEST" ] || { echo "FAIL usage rc=$rc out=$out"; fail=1; }
out=$(bash $impl src2 dst2 2>/dev/null); [ "$out" = "copied 0 files" ] || { echo "FAIL missing src: '$out'"; fail=1; }
mkdir "s p"; touch "s p/x y.txt"; out=$(bash $impl "s p" "d q" 2>/dev/null); [ -f "d q/x y.txt" ] && echo "INFO spaces: handled ($out)" || echo "INFO spaces: NOT handled ($out)"
[ $fail = 0 ] && echo PASS; exit $fail
