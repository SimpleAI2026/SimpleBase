#!/bin/sh

basepath=`dirname $0`
go run $basepath/gen_packages.go > $basepath/../loader/gen_packages.go