# errors

## 1、error: pointer is missing a nullability type specifier (_Nonnull, _Nullable, or _Null_unspecified) [-Werror,-Wnullability-completeness]

answer: https://github.com/golang/go/issues/35247

try: 
```
export CGO_CPPFLAGS="-Wno-error -Wno-nullability-completeness -Wno-expansion-to-defined"
```

## 2、_cgo_export.c:3:10: fatal error: 'stdlib.h' file not found

upgrade golang version, this is error on <= go1.19

## 3、sqlite3vfs_test.go:33: SQL logic error

please use sqlite absolute path