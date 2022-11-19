package api

import (
	"encoding/json"

	"github.com/linkxzhou/gongx/packages/server"
)

func AccountLogin(c server.Context) interface{} {
	r := map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOiI2MzRmNzMzMmNjYWNiOGU2NmI1MjlmNDgiLCJleHAiOjE2Njg4MjkyMzUsImlhdCI6MTY2ODc0MjgzNX0._1pf26on-X0RQFzTPzY6JL1Ny0j_YGevjxKbeFNjTLY",
			"username":     "zhoulv",
			"uid":          "634f7332ccacb8e66b529f48",
			"expire":       1668829235,
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
