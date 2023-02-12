package log

import (
	"testing"

	"github.com/linkxzhou/gongx/packages/log"
)

func TestLevelString(t *testing.T) {
	log.SetLevel(log.LevelDebug)
	log.Debug("This is debug log", "test debug")
	log.Info("This is info log", "test log")
}

func TestLevelInt(t *testing.T) {
	log.SetLevel(log.LevelDebug)
	log.Debug("This is debug log", 1)
	log.Info("This is info log", 1999)
	log.Info("This is info log", 1e10)
}

func TestLevelMap(t *testing.T) {
	log.SetLevel(log.LevelInfo)
	log.Debug("This is debug log", map[string]string{"test debug": "test debug params2"})
	log.Info("This is info log", map[string]string{"test info": "test info params2"})
}
