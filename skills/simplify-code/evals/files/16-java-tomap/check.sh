#!/bin/bash
# usage: check.sh <Index.java>
d=$(mktemp -d); cp "$1" $d/Index.java; cp "$(dirname "$0")/Test.java" $d/
cd $d && javac Index.java Test.java 2>&1 | head -20 && java -cp . Test
