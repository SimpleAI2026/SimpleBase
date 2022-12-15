package api

import (
	"github.com/linkxzhou/gongx/packages/server"
)

func ApplicationMy(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"created": []interface{}{
			map[string]interface{}{
				"_id":           "634f73721e1d2d44d065cca0",
				"name":          "测试1",
				"created_by":    "634f7332ccacb8e66b529f48",
				"appid":         "u3nnf8",
				"status":        "running",
				"collaborators": []interface{}{},
				"runtime": map[string]interface{}{
					"image": "docker.io/lafyun/app-service:0.8.11",
				},
				"buckets": []interface{}{
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
				"packages":   []interface{}{},
				"created_at": "2022-10-19T03:48:01.680Z",
				"updated_at": "2022-10-25T01:58:50.882Z",
				"spec": map[string]interface{}{
					"_id":      "634f73711e1d2d44d065cc9f",
					"appid":    "u3nnf8",
					"start_at": "2022-10-19T03:48:01.680Z",
					"end_at":   "2099-12-31T00:00:00.000Z",
					"enabled":  true,
					"spec": map[string]interface{}{
						"_id":               "627103c09e4d81d1bb1ff591",
						"name":              "通用",
						"label":             "1C1G",
						"request_cpu":       5,
						"request_memory":    67108864,
						"limit_cpu":         500,
						"limit_memory":      134217728,
						"database_capacity": 1073741824,
						"storage_capacity":  3221225472,
						"bandwith":          10485760,
						"out_traffic":       2147483648,
						"priority":          0,
						"enabled":           true,
						"created_at":        "2022-05-03T10:28:16.424Z",
						"updated_at":        "2022-05-03T10:28:16.424Z",
					},
					"created_at": "2022-10-19T03:48:01.682Z",
					"updated_at": "2022-10-19T03:48:01.682Z",
				},
			},
		},
		"joined": []interface{}{
			map[string]interface{}{
				"_id":           "634f73721e1d2d44d065cca0",
				"name":          "测试1",
				"created_by":    "634f7332ccacb8e66b529f48",
				"appid":         "u3nnf8",
				"status":        "running",
				"collaborators": []interface{}{},
				"runtime": map[string]interface{}{
					"image": "docker.io/lafyun/app-service:0.8.11",
				},
				"buckets": []interface{}{
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
				"packages":   []interface{}{},
				"created_at": "2022-10-19T03:48:01.680Z",
				"updated_at": "2022-10-25T01:58:50.882Z",
				"spec": map[string]interface{}{
					"_id":      "634f73711e1d2d44d065cc9f",
					"appid":    "u3nnf8",
					"start_at": "2022-10-19T03:48:01.680Z",
					"end_at":   "2099-12-31T00:00:00.000Z",
					"enabled":  true,
					"spec": map[string]interface{}{
						"_id":               "627103c09e4d81d1bb1ff591",
						"name":              "通用",
						"label":             "1C1G",
						"request_cpu":       5,
						"request_memory":    67108864,
						"limit_cpu":         500,
						"limit_memory":      134217728,
						"database_capacity": 1073741824,
						"storage_capacity":  3221225472,
						"bandwith":          10485760,
						"out_traffic":       2147483648,
						"priority":          0,
						"enabled":           true,
						"created_at":        "2022-05-03T10:28:16.424Z",
						"updated_at":        "2022-05-03T10:28:16.424Z",
					},
					"created_at": "2022-10-19T03:48:01.682Z",
					"updated_at": "2022-10-19T03:48:01.682Z",
				},
			},
		},
	}

	return CommonResp(code, message, data)
}

func ApplicationSpecs(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := []interface{}{
		map[string]interface{}{
			"_id":               "627103c09e4d81d1bb1ff591",
			"name":              "通用",
			"label":             "1C1G",
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

	return CommonResp(code, message, data)
}

func ApplicationCreate(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"appid":      "u3nnf8",
		"created_at": "2022-05-03T10:28:16.424Z",
		"updated_at": "2022-05-03T10:28:16.424Z",
	}

	return CommonResp(code, message, data)
}

func ApplicationUpdate(c server.Context) interface{} {
	code := 0
	message := emptyString

	return CommonResp(code, message, nil)
}

func ApplicationRemove(c server.Context) interface{} {
	code := 0
	message := emptyString

	return CommonResp(code, message, nil)
}

func ApplicationInstanceOp(c server.Context) interface{} {
	code := 0
	message := emptyString

	return CommonResp(code, message, nil)
}
