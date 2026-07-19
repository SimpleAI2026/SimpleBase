package log

import (
	"os"
	"time"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LevelEnableFunc func(lvl Level) bool

type RotateOption struct {
	MaxSize    int    `mapstructure:"max-size" yaml:"max-size"`
	MaxAge     int    `mapstructure:"max-age" yaml:"max-age"`
	MaxBackups int    `mapstructure:"max-backups" yaml:"max-backups"`
	Compress   bool   `mapstructure:"compress" yaml:"compress"`
	Level      string `mapstructure:"level" yaml:"level"`
}

type TeeOption struct {
	Filename string
	Lef      LevelEnableFunc
	Ropt     RotateOption
}

func NewTee(topts []TeeOption, opts ...Option) *Logger {
	var zcores []zapcore.Core
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02T15:04:05.000"))
	}

	for _, topt := range topts {
		topt := topt

		lv := zap.LevelEnablerFunc(func(lvl Level) bool {
			return topt.Lef(lvl)
		})

		file, err := os.OpenFile(topt.Filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}

		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(cfg.EncoderConfig),
			zapcore.AddSync(file),
			lv,
		)
		zcores = append(zcores, core)
	}

	opts = append(opts, WithCaller(true))
	logger := &Logger{
		l: zap.New(zapcore.NewTee(zcores...), opts...),
	}
	return logger
}

func NewTeeWithRotate(topts []TeeOption, consoleLevel Level, opts ...Option) *Logger {
	var zcores []zapcore.Core
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.Format("2006-01-02T15:04:05.000Z0700"))
	}

	for _, topt := range topts {
		topt := topt
		lv := zap.LevelEnablerFunc(topt.Lef)

		w := zapcore.AddSync(&lumberjack.Logger{
			Filename:   topt.Filename,
			MaxSize:    topt.Ropt.MaxAge,
			MaxBackups: topt.Ropt.MaxBackups,
			MaxAge:     topt.Ropt.MaxAge,
			Compress:   topt.Ropt.Compress,
			LocalTime:  true,
		})

		core := zapcore.NewCore(
			//zapcore.NewJSONEncoder(cfg.EncoderConfig),
			zapcore.NewConsoleEncoder(cfg.EncoderConfig),
			zapcore.AddSync(w),
			lv,
		)
		zcores = append(zcores, core)
	}
	core := zapcore.NewCore(
		//zapcore.NewJSONEncoder(cfg.EncoderConfig),
		zapcore.NewConsoleEncoder(cfg.EncoderConfig),
		zapcore.AddSync(os.Stderr),
		zapcore.Level(consoleLevel),
	)
	zcores = append(zcores, core)
	opts = append(opts, WithCaller(true))
	logger := &Logger{
		l: zap.New(zapcore.NewTee(zcores...), opts...),
	}
	return logger
}
