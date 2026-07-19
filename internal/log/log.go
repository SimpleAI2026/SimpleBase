package log

import (
	"io"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Level = zapcore.Level

const (
	InfoLevel   Level = zap.InfoLevel
	WarnLevel   Level = zap.WarnLevel
	ErrorLevel  Level = zap.ErrorLevel
	DPanicLevel Level = zap.DPanicLevel
	PanicLevel  Level = zap.PanicLevel // log msg, then panic
	FatalLevel  Level = zap.FatalLevel // log msg, then calls os.Exit(1)
	DebugLevel  Level = zap.DebugLevel
)

type Field = zap.Field

type Logger struct {
	l     *zap.Logger // zap确保zap.Logger是多协程安全的
	level Level
}

var std = New(os.Stderr, InfoLevel)
var sugarStd = std.l.Sugar()

func Default() *Logger {
	return std
}

func ResetDefault(l *Logger) {
	std = l
	Info = std.l.Info
	Warn = std.l.Warn
	Error = std.l.Error
	DPanic = std.l.DPanic
	Panic = std.l.Panic
	Fatal = std.l.Fatal
	Debug = std.l.Debug

	sugarStd = std.l.Sugar()
	SugarDebug = sugarStd.Debug
	SugarInfo = sugarStd.Info
	SugarWarn = sugarStd.Warn
	SugarError = sugarStd.Error
	SugarDPanic = sugarStd.DPanic
	SugarPanic = sugarStd.Panic
	SugarFatal = sugarStd.Fatal

	SugarDebugf = sugarStd.Debugf
	SugarInfof = sugarStd.Infof
	SugarWarnf = sugarStd.Warnf
	SugarErrorf = sugarStd.Errorf
	SugarDPanicf = sugarStd.DPanicf
	SugarPanicf = sugarStd.Panicf
	SugarFatalf = sugarStd.Fatalf

	SugarDebugw = sugarStd.Debugw
	SugarInfow = sugarStd.Infow
	SugarWarnw = sugarStd.Warnw
	SugarErrorw = sugarStd.Errorw
	SugarDPanicw = sugarStd.DPanicw
	SugarPanicw = sugarStd.Panicw
	SugarFatalw = sugarStd.Fatalw
}

type Option = zap.Option

var (
	WithCaller    = zap.WithCaller
	AddStacktrace = zap.AddStacktrace
	AddCallerSkip = zap.AddCallerSkip // 打印文件skip，多个skip则相加
)

func New(writer io.Writer, level Level, opts ...Option) *Logger {
	if writer == nil {
		panic("writer is nil")
	}

	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02T15:04:05.000Z0700"))
	}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(cfg.EncoderConfig),
		// zapcore.NewConsoleEncoder(cfg.EncoderConfig),
		zapcore.AddSync(writer),
		zapcore.Level(level),
	)

	opts = append(opts, WithCaller(true))
	logger := &Logger{
		l:     zap.New(core, opts...),
		level: level,
	}

	return logger
}

func (l *Logger) Sync() error {
	return l.l.Sync()
}

func Sync() error {
	if std != nil {
		return std.Sync()
	}
	return nil
}

func (l *Logger) Debug(msg string, field ...Field) {
	l.l.Debug(msg, field...)
}

func (l *Logger) Info(msg string, field ...Field) {
	l.l.Info(msg, field...)
}

func (l *Logger) Warn(msg string, field ...Field) {
	l.l.Warn(msg, field...)
}

func (l *Logger) Error(msg string, field ...Field) {
	l.l.Error(msg, field...)
}

func (l *Logger) DPanic(msg string, field ...Field) {
	l.l.DPanic(msg, field...)
}

func (l *Logger) Panic(msg string, field ...Field) {
	l.l.Panic(msg, field...)
}

func (l *Logger) Fatal(msg string, field ...Field) {
	l.l.Fatal(msg, field...)
}

var (
	Skip        = zap.Skip
	Binary      = zap.Binary
	Bool        = zap.Bool
	Boolp       = zap.Boolp
	ByteString  = zap.ByteString
	Complex128  = zap.Complex128
	Complex128p = zap.Complex128p
	Complex64   = zap.Complex64
	Complex64p  = zap.Complex64p
	Float64     = zap.Float64
	Float64p    = zap.Float64p
	Float32     = zap.Float32
	Float32p    = zap.Float32p
	Int         = zap.Int
	Intp        = zap.Intp
	Int64       = zap.Int64
	Int64p      = zap.Int64p
	Int32       = zap.Int32
	Int32p      = zap.Int32
	Int16       = zap.Int16
	Int16p      = zap.Int16p
	Int8        = zap.Int8
	Int8p       = zap.Int8p
	String      = zap.String
	Stringp     = zap.Stringp
	Uint        = zap.Uint
	Uintp       = zap.Uintp
	Uint64      = zap.Uint64
	Uint64p     = zap.Uint64p
	Uint32      = zap.Uint32
	Uint32p     = zap.Uint32p
	Uint16      = zap.Uint16
	Uint16p     = zap.Uint16p
	Uint8       = zap.Uint8
	Uint8p      = zap.Uint8p
	Uintptr     = zap.Uintptr
	Uintptrp    = zap.Uintptr
	Reflect     = zap.Reflect
	Namespace   = zap.Namespace
	Stringer    = zap.Stringer
	Time        = zap.Time
	Timep       = zap.Timep
	Stack       = zap.Stack
	StackSkip   = zap.StackSkip
	Duration    = zap.Duration
	Durationp   = zap.Durationp

	Object = zap.Object
	Inline = zap.Inline
	Any    = zap.Any
)

var (
	Info   = std.l.Info
	Warn   = std.l.Warn
	Error  = std.l.Error
	DPanic = std.l.DPanic
	Panic  = std.l.Panic
	Fatal  = std.l.Fatal
	Debug  = std.l.Debug

	SugarDebug  = sugarStd.Debug
	SugarInfo   = sugarStd.Info
	SugarWarn   = sugarStd.Warn
	SugarError  = sugarStd.Error
	SugarDPanic = sugarStd.DPanic
	SugarPanic  = sugarStd.Panic
	SugarFatal  = sugarStd.Fatal

	SugarDebugf  = sugarStd.Debugf
	SugarInfof   = sugarStd.Infof
	SugarWarnf   = sugarStd.Warnf
	SugarErrorf  = sugarStd.Errorf
	SugarDPanicf = sugarStd.DPanicf
	SugarPanicf  = sugarStd.Panicf
	SugarFatalf  = sugarStd.Fatalf

	SugarDebugw  = sugarStd.Debugw
	SugarInfow   = sugarStd.Infow
	SugarWarnw   = sugarStd.Warnw
	SugarErrorw  = sugarStd.Errorw
	SugarDPanicw = sugarStd.DPanicw
	SugarPanicw  = sugarStd.Panicw
	SugarFatalw  = sugarStd.Fatalw
)
