package api

import (
	"encoding/json"

	"github.com/linkxzhou/gongx/packages/server"
)

func OssBuckets(c server.Context) interface{} {
	r := map[string]interface{}{
		"code": 0,
		"data": []interface{}{
			map[string]interface{}{
				"name":  "data",
				"mode":  "public-read",
				"quota": 1073741824,
			}, map[string]interface{}{
				"name":  "public",
				"mode":  "public-read-write",
				"quota": 1073741824,
			},
		},
	}

	if r1, err := json.Marshal(r); err != nil {
		return nil
	} else {
		return string(r1)
	}
}

func OssWebsites(c server.Context) interface{} {
	r := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{
				"_id":         "63575ee9b151ccd9cd674247",
				"label":       "纯纯粹粹",
				"bucket_name": "data",
				"appid":       "u3nnf8",
				"domain":      []interface{}{},
				"cname":       "u3nnf8-data.oss.lafyun.com",
				"status":      "enabled",
				"state":       "created",
				"created_at":  "2022-10-25T03:58:33.810Z",
				"updated_at":  "2022-10-25T03:58:33.810Z",
			},
		},
	}

	if r1, err := json.Marshal(r); err != nil {
		return nil
	} else {
		return string(r1)
	}
}
