package api

import (
	"encoding/json"

	"github.com/linkxzhou/gongx/packages/server"
)

func AppsMy(c server.Context) interface{} {
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

func AppsSpecs(c server.Context) interface{} {
	r := map[string]interface{}{
		"data": map[string]interface{}{
			"_id":               "627103c09e4d81d1bb1ff591",
			"name":              "starter",
			"label":             "starter",
			"request_cpu":       5,
			"request_memory":    67108864,
			"limit_cpu":         500,
			"limit_memory":      268435456,
			"database_capacity": 1073741824,
			"storage_capacity":  3221225472,
			"bandwith":          10485760,
			"out_traffic":       2147483648,
			"priority":          0,
			"enabled":           true,
			"created_at":        "2022-05-03T10:28:16.424Z",
			"updated_at":        "2022-05-03T10:28:16.424Z",
		},
	}

	if r1, err := json.Marshal(r); err != nil {
		return nil
	} else {
		return string(r1)
	}
}
