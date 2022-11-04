#!/bin/sh

basepath=`dirname $0`
go run $basepath/tool.go > $basepath/../loader/gen_packages.go