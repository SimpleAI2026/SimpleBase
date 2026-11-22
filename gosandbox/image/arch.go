package image

import "runtime"

// runtimeArch 是 runtime.GOARCH 的可替换包装（测试里换掉）。
var runtimeArch = func() string { return runtime.GOARCH }
