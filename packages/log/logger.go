package log

import (
	"fmt"

	"go.uber.org/zap"
)

// Level log level
type Level int

// log level const
const (
	LevelNil Level = iota
	LevelTrace
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var (
	defaultLogger *zap.Logger
	defaultLevel  Level = LevelInfo
)

func init() {
	defaultLogger = zap.NewExample()
}

func getLogMsg(args ...interface{}) string {
	msg := fmt.Sprint(args...)
	return msg
}

func getLogMsgf(format string, args ...interface{}) string {
	msg := fmt.Sprintf(format, args...)
	return msg
}

func SetLogger(logger *zap.Logger) {
	defaultLogger = logger
}

// Debug logs to DEBUG log. Arguments are handled in the manner of fmt.Print.
func Debug(args ...interface{}) {
	if defaultLevel <= LevelDebug {
		defaultLogger.Debug(getLogMsg(args...))
	}
}

// Debugf logs to DEBUG log. Arguments are handled in the manner of fmt.Printf.
func Debugf(format string, args ...interface{}) {
	if defaultLevel <= LevelDebug {
		defaultLogger.Debug(getLogMsgf(format, args...))
	}
}

// Info logs to INFO log. Arguments are handled in the manner of fmt.Print.
func Info(args ...interface{}) {
	if defaultLevel <= LevelInfo {
		defaultLogger.Info(getLogMsg(args...))
	}
}

// Infof logs to INFO log. Arguments are handled in the manner of fmt.Printf.
func Infof(format string, args ...interface{}) {
	if defaultLevel <= LevelInfo {
		defaultLogger.Info(getLogMsgf(format, args...))
	}
}

// Warn logs to WARNING log. Arguments are handled in the manner of fmt.Print.
func Warn(args ...interface{}) {
	if defaultLevel <= LevelWarn {
		defaultLogger.Warn(getLogMsg(args...))
	}
}

// Warnf logs to WARNING log. Arguments are handled in the manner of fmt.Printf.
func Warnf(format string, args ...interface{}) {
	if defaultLevel <= LevelWarn {
		defaultLogger.Warn(getLogMsgf(format, args...))
	}
}

// Error logs to ERROR log. Arguments are handled in the manner of fmt.Print.
func Error(args ...interface{}) {
	if defaultLevel <= LevelError {
		defaultLogger.Warn(getLogMsg(args...))
	}
}

// Errorf logs to ERROR log. Arguments are handled in the manner of fmt.Printf.
func Errorf(format string, args ...interface{}) {
	if defaultLevel <= LevelError {
		defaultLogger.Warn(getLogMsgf(format, args...))
	}
}

// Fatal logs to ERROR log. Arguments are handled in the manner of fmt.Print.
// that all Fatal logs will exit with os.Exit(1).
// Implementations may also call os.Exit() with a non-zero exit code.
func Fatal(args ...interface{}) {
	if defaultLevel <= LevelFatal {
		defaultLogger.Fatal(getLogMsg(args...))
	}
}

// Fatalf logs to ERROR log. Arguments are handled in the manner of fmt.Printf.
func Fatalf(format string, args ...interface{}) {
	if defaultLevel <= LevelFatal {
		defaultLogger.Warn(getLogMsgf(format, args...))
	}
}

func SetLevel(level Level) {
	defaultLevel = level
}

func Sync() {
	defaultLogger.Sync()
}
