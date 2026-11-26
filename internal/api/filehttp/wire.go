// 文件说明：wire.go — 站点管理接口响应的线上形态别名（导出仅供路由级测试解码断言）
//
// 用法
//
//	var info filehttp.SiteInfoDTO   // 解码 GET /v1/projects/:id/site 的响应
//
// 调用架构
//
//	[上游] internal/api 的路由级测试
//	         │ 使用本包导出类型
//	         ▼
//	[本文件] internal/api/filehttp/wire.go
//	         │ 依赖
//	         ▼
//	[下游] 仅本包
//
// 主要符号
//
//	type SiteInfoDTO = siteInfoDTO
//	type PublicPathDTO = publicPathDTO
//	type S3ObjectDTO = s3ObjectDTO
//	type S3FolderMetaDTO = s3FolderMetaDTO

package filehttp

type (
	// SiteInfoDTO 是站点信息接口的响应形态。
	SiteInfoDTO = siteInfoDTO
	// PublicPathDTO 是公开路径条目的响应形态。
	PublicPathDTO = publicPathDTO
	// S3ObjectDTO 是对象列表 / 上传响应的对象元数据形态。
	S3ObjectDTO = s3ObjectDTO
	// S3FolderMetaDTO 是文件夹元数据形态。
	S3FolderMetaDTO = s3FolderMetaDTO
)
