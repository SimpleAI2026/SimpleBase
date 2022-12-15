package api

import (
	"github.com/linkxzhou/gongx/packages/server"
)

func AppsMy(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOiI2MzRmNzMzMmNjYWNiOGU2NmI1MjlmNDgiLCJleHAiOjE2Njg4MjkyMzUsImlhdCI6MTY2ODc0MjgzNX0._1pf26on-X0RQFzTPzY6JL1Ny0j_YGevjxKbeFNjTLY",
		"username":     "zhoulv",
		"uid":          "634f7332ccacb8e66b529f48",
		"expire":       1668829235,
	}

	return CommonResp(code, message, data)
}

func AppsSpecs(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
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
	}

	return CommonResp(code, message, data)
}

func AppsInfo(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"application": map[string]interface{}{
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
				},
				map[string]interface{}{
					"name":  "public",
					"mode":  "public-read-write",
					"quota": 1073741824,
				},
			},
			"packages":   []interface{}{},
			"created_at": "2022-10-19T03:48:01.680Z",
			"updated_at": "2022-10-25T01:58:50.882Z",
		},
		"permissions": []string{
			"fn:ListFunctions", "fn:GetFunction", "fn:CreateFunction", "fn:UpdateFunction", "fn:DeleteFunction", "fn:InvokeFunction", "fn:PublishFunction", "fn:ListLogs", "fn:ListPackages", "fn:CreatePackage", "fn:UpdatePackage", "fn:DeletePackage", "db:ListCollections", "db:GetCollection", "db:CreateCollection", "db:UpdateCollection", "db:DeleteCollection", "db:ListDocuments", "db:GetDocument", "db:CreateDocument", "db:UpdateDocument", "db:DeleteDocument", "db:ListPolicies", "db:GetPolicy", "db:CreatePolicy", "db:UpdatePolicy", "db:DeletePolicy", "db:PublishPolicy", "oss:ListBuckets", "oss:GetBucket", "oss:CreateBucket", "oss:UpdateBucket", "oss:DeleteBucket", "oss:CreateServiceAccount", "rep:ListReplicateAuth", "rep:GetReplicateAuth", "rep:CreateReplicateAuth", "rep:UpdateReplicateAuth", "rep:DeleteReplicateAuth", "rep:ListReplicateRequest", "rep:GetReplicateRequest", "rep:CreateReplicateRequest", "rep:UpdateReplicateRequest", "rep:DeleteReplicateRequest", "app:ListApplications", "app:GetApplication", "app:CreateApplication", "app:UpdateApplication", "app:DeleteApplication", "app:StartInstance", "app:StopInstance", "web:ListWebsites", "web:GetWebsite", "web:CreateWebsite", "web:UpdateWebsite", "web:DeleteWebsite",
		},
		"roles":                 []string{"Admin"},
		"debug_token":           "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhcHBpZCI6InUzbm5mOCIsInR5cGUiOiJkZWJ1ZyIsImV4cCI6MTY2OTYzODI5MiwiaWF0IjoxNjY5NTUxODkyfQ.jQ_eo7JBDWMSTzMHd4CdY1cATs7P1OP_aIrnJcVvExA",
		"app_deploy_host":       "lafyun.com:443",
		"app_deploy_url_schema": "https",
		"oss_external_endpoint": "https://oss.lafyun.com",
		"oss_internal_endpoint": "http://oss.minio:9000",
		"spec": map[string]interface{}{
			"_id":      "634f73711e1d2d44d065cc9f",
			"appid":    "u3nnf8",
			"start_at": "2022-10-19T03:48:01.680Z",
			"end_at":   "2099-12-31T00:00:00.000Z",
			"enabled":  true,
			"spec": map[string]interface{}{
				"_id":               "627103c09e4d81d1bb1ff591",
				"name":              "starter",
				"label":             "starter",
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
	}

	return CommonResp(code, message, data)
}
