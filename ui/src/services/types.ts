/**
 * 统一的后端接口类型定义与 API 抽象的唯一入口（契约以 internal/api/router.go 与 error.go 为准）。
 * 按域拆在 `types/`：platform（项目 / 认证 / S3 / 日志 / 云函数 / 定时 / 沙盒）、data（库 / SQL / 集合 / KV）、
 * agent（云助手 / 协作 / MCP）、api（`Api` 接口）。全仓仍从这里或 `services/api` 引入，不直接 import 子文件。
 */
export * from '@/services/types/platform'
export * from '@/services/types/data'
export * from '@/services/types/agent'
export * from '@/services/types/api'
