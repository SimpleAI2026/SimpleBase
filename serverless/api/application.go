package api

import (
	"github.com/linkxzhou/gongx/packages/server"
)

func Application(c server.Context) interface{} {
	return &BaseResponse{
		ErrCode:    0,
		ErrMessage: "OK",
	}
}
