package api

import (
	"github.com/linkxzhou/goedge/packages/server"
)

func AccountLogin(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1aWQiOiI2MzRmNzMzMmNjYWNiOGU2NmI1MjlmNDgiLCJleHAiOjE2NzA4Mzc5MDAsImlhdCI6MTY3MDc1MTUwMH0.cZwxRHWjrKaQWHQUzsrFSfO5-3BtFPCq-DBL9lVMiWs",
		"username":     "zhoulv",
		"uid":          "634f7332ccacb8e66b529f48",
		"expire":       1680837900,
	}

	return CommonResp(code, message, data)
}

func AccountProfile(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"_id":        "634f7332ccacb8e66b529f48",
		"username":   "zhoulv",
		"phone":      "17724490919",
		"quota":      map[string]interface{}{"app_count": 1},
		"name":       "zhoulv",
		"created_at": "2022-10-19T03:46:58.422Z",
		"updated_at": "2022-10-19T03:46:58.422Z",
	}

	return CommonResp(code, message, data)
}

func AccountSignup(c server.Context) interface{} {
	return CommonResp(0, emptyString, nil)
}

func CollaboratorsList(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := []interface{}{
		map[string]interface{}{
			"uid": "1111",
			"user": map[string]interface{}{
				"username": "111",
				"name":     "1111",
			},
			"roles": []interface{}{
				"FunctionReadyOnly",
			},
		},
	}

	return CommonResp(code, message, data)
}

func CollaboratorsRoles(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := []interface{}{
		map[string]interface{}{
			"name":    "FunctionReadyOnly",
			"label":   "Function Ready Only",
			"actions": []string{"fn:ListFunctions", "fn:GetFunction", "fn:ListLogs", "fn:ListPackages"},
		}, map[string]interface{}{
			"name":    "FunctionFullAccess",
			"label":   "Function Full Access",
			"actions": []string{"fn:ListFunctions", "fn:GetFunction", "fn:CreateFunction", "fn:UpdateFunction", "fn:DeleteFunction", "fn:InvokeFunction", "fn:PublishFunction", "fn:ListLogs", "fn:ListPackages", "fn:CreatePackage", "fn:UpdatePackage", "fn:DeletePackage"},
		}, map[string]interface{}{
			"name":    "DatabaseReadyOnly",
			"label":   "Database Ready Only",
			"actions": []string{"db:ListCollections", "db:GetCollection", "db:ListDocuments", "db:GetDocument", "db:ListPolicies", "db:GetPolicy"},
		}, map[string]interface{}{
			"name":    "DatabaseFullAccess",
			"label":   "Database Full Access",
			"actions": []string{"db:ListCollections", "db:GetCollection", "db:CreateCollection", "db:UpdateCollection", "db:DeleteCollection", "db:ListDocuments", "db:GetDocument", "db:CreateDocument", "db:UpdateDocument", "db:DeleteDocument", "db:ListPolicies", "db:GetPolicy", "db:CreatePolicy", "db:UpdatePolicy", "db:DeletePolicy", "db:PublishPolicy"},
		}, map[string]interface{}{
			"name":    "StorageReadOnly",
			"label":   "Storage Read Only",
			"actions": []string{"oss:ListBuckets", "oss:GetBucket"},
		}, map[string]interface{}{
			"name":    "StorageFullAccess",
			"label":   "Storage Full Access",
			"actions": []string{"oss:ListBuckets", "oss:GetBucket", "oss:CreateBucket", "oss:UpdateBucket", "oss:DeleteBucket", "oss:CreateServiceAccount"},
		}, map[string]interface{}{
			"name":    "ReplicationReadOnly",
			"label":   "Replication Read Only",
			"actions": []string{"rep:ListReplicateAuth", "rep:GetReplicateAuth", "rep:ListReplicateRequest", "rep:GetReplicateRequest"},
		}, map[string]interface{}{
			"name":    "ReplicationFullAccess",
			"label":   "Replication Full Access",
			"actions": []string{"rep:ListReplicateAuth", "rep:GetReplicateAuth", "rep:CreateReplicateAuth", "rep:UpdateReplicateAuth", "rep:DeleteReplicateAuth", "rep:ListReplicateRequest", "rep:GetReplicateRequest", "rep:CreateReplicateRequest", "rep:UpdateReplicateRequest", "rep:DeleteReplicateRequest"},
		}, map[string]interface{}{
			"name":    "ApplicationReadOnly",
			"label":   "Application Read Only",
			"actions": []string{"app:ListApplications", "app:GetApplication"},
		}, map[string]interface{}{
			"name":    "InstanceOperator",
			"label":   "Instance Operator",
			"actions": []string{"app:StartInstance", "app:StopInstance"},
		}, map[string]interface{}{
			"name":    "ApplicationFullAccess",
			"label":   "Application Full Access",
			"actions": []string{"app:ListApplications", "app:GetApplication", "app:CreateApplication", "app:UpdateApplication", "app:DeleteApplication", "app:StartInstance", "app:StopInstance"},
		}, map[string]interface{}{
			"name":    "WebsiteReadOnly",
			"label":   "Website Read Only",
			"actions": []string{"web:ListWebsites", "web:GetWebsite"},
		}, map[string]interface{}{
			"name":    "WebsiteFullAccess",
			"label":   "Website Full Access",
			"actions": []string{"web:ListWebsites", "web:GetWebsite", "web:CreateWebsite", "web:UpdateWebsite", "web:DeleteWebsite"},
		}, map[string]interface{}{
			"name":    "Admin",
			"label":   "Admin",
			"actions": []string{"fn:ListFunctions", "fn:GetFunction", "fn:CreateFunction", "fn:UpdateFunction", "fn:DeleteFunction", "fn:InvokeFunction", "fn:PublishFunction", "fn:ListLogs", "fn:ListPackages", "fn:CreatePackage", "fn:UpdatePackage", "fn:DeletePackage", "db:ListCollections", "db:GetCollection", "db:CreateCollection", "db:UpdateCollection", "db:DeleteCollection", "db:ListDocuments", "db:GetDocument", "db:CreateDocument", "db:UpdateDocument", "db:DeleteDocument", "db:ListPolicies", "db:GetPolicy", "db:CreatePolicy", "db:UpdatePolicy", "db:DeletePolicy", "db:PublishPolicy", "oss:ListBuckets", "oss:GetBucket", "oss:CreateBucket", "oss:UpdateBucket", "oss:DeleteBucket", "oss:CreateServiceAccount", "rep:ListReplicateAuth", "rep:GetReplicateAuth", "rep:CreateReplicateAuth", "rep:UpdateReplicateAuth", "rep:DeleteReplicateAuth", "rep:ListReplicateRequest", "rep:GetReplicateRequest", "rep:CreateReplicateRequest", "rep:UpdateReplicateRequest", "rep:DeleteReplicateRequest", "app:ListApplications", "app:GetApplication", "app:CreateApplication", "app:UpdateApplication", "app:DeleteApplication", "app:StartInstance", "app:StopInstance", "web:ListWebsites", "web:GetWebsite", "web:CreateWebsite", "web:UpdateWebsite", "web:DeleteWebsite"},
		},
	}

	return CommonResp(code, message, data)
}

func CollaboratorsSearch(c server.Context) interface{} {
	code := 0
	message := emptyString
	data := map[string]interface{}{
		"uid":      "1111",
		"username": "111",
		"name":     "1111",
	}

	return CommonResp(code, message, data)
}

func CollaboratorsInvite(c server.Context) interface{} {
	code := 0
	message := emptyString

	return CommonResp(code, message, nil)
}
