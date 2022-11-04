package log

import (
	"testing"

	"github.com/linkxzhou/gongx/log"
)

func TestLevel_String(t *testing.T) {
	log.SetLevel(log.LevelInfo)
	log.Debug("=================== Debug", "1111")
	log.Info("=================== Info", "2222")
}

func TestLevel_Int(t *testing.T) {
	log.SetLevel(log.LevelInfo)
	log.Debug("=================== Debug", 1111)
	log.Info("=================== Info", 2222)
}

func TestLevel_Map(t *testing.T) {
	log.SetLevel(log.LevelInfo)
	log.Debug("=================== Debug", map[string]string{"1111": "1111"})
	log.Info("=================== Info", map[string]string{"2222": "2222"})
}
