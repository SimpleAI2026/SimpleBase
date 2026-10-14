#!/bin/bash
d=$(mktemp -d); here=$(cd "$(dirname "$0")" && pwd)
g++ -std=c++17 -fsanitize=address -g -DIMPL="\"$(readlink -f $1)\"" "$here/test.cpp" -o $d/t 2>$d/err && $d/t || { echo FAIL; head -20 $d/err; exit 1; }
