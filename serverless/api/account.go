package api

import (
	"encoding/json"

	"github.com/linkxzhou/gongx/packages/server"
)

func AccountLogin(c server.Context) interface{} {
	r := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOiI2MzRmNzMzMmNjYWNiOGU2NmI1MjlmNDgiLCJleHAiOjE2NzA4Mzc5MDAsImlhdCI6MTY3MDc1MTUwMH0.cZwxRHWjrKaQWHQUzsrFSfO5-3BtFPCq-DBL9lVMiWs",
			"username": "zhoulv",
			"uid": "634f7332ccacb8e66b529f48",
			"expire": 1670837900,
		},
	}

	if r1, err := json.Marshal(r); err != nil {
		return nil
	} else {
		return string(r1)
	}
}

func AccountProfile(c server.Context) interface{} {
	r := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"_id":        "634f7332ccacb8e66b529f48",
			"username":   "zhoulv",
			"phone":      "17724490919",
			"quota":      map[string]interface{}{"app_count": 1},
			"name":       "zhoulv",
			"created_at": "2022-10-19T03:46:58.422Z",
			"updated_at": "2022-10-19T03:46:58.422Z",
		},
	}

	if r1, err := json.Marshal(r); err != nil {
		return nil
	} else {
		return string(r1)
	}
}

func AccountSignup(c server.Context) interface{} {
	r := map[string]interface{}{
		"code": 0,
	}

	if r1, err := json.Marshal(r); err != nil {
		return nil
	} else {
		return string(r1)
	}
}