#!/bin/sh

basepath=`dirname $0`
go run $basepath/*.go ImportPkgs > $basepath/../loader/gen_packages.go
go run $basepath/*.go OpArithmetic > $basepath/../gen_arithmetic.go
go run $basepath/*.go OpShift > $basepath/../gen_shift.go
go run $basepath/*.go OpCvt > $basepath/../gen_cvt.go
go run $basepath/*.go OpUn > $basepath/../gen_un.go