#!/bin/sh

basepath=`dirname $0`
go run $basepath/*.go ImportPkgs > $basepath/../importer/autogen_packages.go
go run $basepath/*.go OpArithmetic > $basepath/../autogen_arithmetic.go
go run $basepath/*.go OpShift > $basepath/../autogen_shift.go
go run $basepath/*.go OpCvt > $basepath/../autogen_cvt.go
go run $basepath/*.go OpUn > $basepath/../autogen_un.go