package log

import (
	"testing"

	"github.com/linkxzhou/gongx/log"
)

func TestLevel_String(t *testing.T) {
	log.SetLevel(log.LevelInfo)
	log.Debug("=================== Debug")
	log.Info("=================== Info")
}
