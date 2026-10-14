#!/bin/bash
d=$(mktemp -d); here=$(cd "$(dirname "$0")" && pwd); impl=$(readlink -f "$1")
# impl may be a single file or a directory of .java files
if [ -d "$impl" ]; then cp "$impl"/*.java $d/; else cp "$impl" $d/OrderManager.java; fi
cp "$here/Test.java" $d/; cd $d && javac *.java 2>err && java Test || { echo FAIL; head err; exit 1; }
