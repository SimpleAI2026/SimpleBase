package sshd

import "os"

// writeGarbage 把一个无法解析的内容写到密钥文件上，用来验证损坏文件不会被静默覆盖。
func writeGarbage(path string) error { return os.WriteFile(path, []byte("not a key"), 0o600) }
