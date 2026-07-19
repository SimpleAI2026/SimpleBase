package log

import (
	"log"
	"path"
)

// LogConfig 日志参数
type LogConfig struct {
	Path         string        `mapstructure:"path" yaml:"path"`
	ConsoleLevel string        `mapstructure:"console-level" yaml:"console-level"`
	AccessOption *RotateOption `mapstructure:"access-option" yaml:"access-option"`
	ErrorOption  *RotateOption `mapstructure:"error-option" yaml:"error-option"`
}

// SetDefaultLogMode 设置默认大日志模式
func SetDefaultLogMode(config *LogConfig) {
	var consoleLevel Level
	var accessLogLevel Level
	var errorLogLevel Level
	err := consoleLevel.Set(config.ConsoleLevel)
	if err != nil {
		log.Fatalf("consoleLevel set error:%v", err)
	}
	err = accessLogLevel.Set(config.AccessOption.Level)
	if err != nil {
		log.Fatalf("accessLogLevel set error:%v", err)
	}
	err = errorLogLevel.Set(config.ErrorOption.Level)
	if err != nil {
		log.Fatalf("errorLogLevel set error:%v", err)
	}
	var tops = []TeeOption{
		{
			Filename: path.Join(config.Path, "access.log"),
			Ropt:     *config.AccessOption,
			Lef: func(lvl Level) bool {
				return lvl <= accessLogLevel
			},
		},
		{
			Filename: path.Join(config.Path, "error.log"),
			Ropt:     *config.ErrorOption,
			Lef: func(lvl Level) bool {
				return lvl > errorLogLevel
			},
		},
	}

	logger := NewTeeWithRotate(tops, consoleLevel)
	ResetDefault(logger)
}
