package api

import (
	"github.com/linkxzhou/goedge/packages/server"
)

func OssBuckets(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := []interface{}{
		map[string]interface{}{
			"name":  "data",
			"mode":  "public-read",
			"quota": 1073741824,
		}, map[string]interface{}{
			"name":  "public",
			"mode":  "public-read-write",
			"quota": 1073741824,
		},
	}

	return CommonResp(code, message, data)
}

func OssWebsites(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := []interface{}{
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
	}

	return CommonResp(code, message, data)
}
