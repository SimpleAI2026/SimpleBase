//go:build !linux

package host

func landlockABI() int       { return 0 }
func seccompSupported() bool { return false }
func detectCgroup() string   { return cgroupNone }
