# Tasks

## 1. 服务端阶段契约

- [ ] 1.1 `starter_template` 白名单校验（目标通用助手、`database:admin`、可写、skills CLI、非系统项目），拒绝返回 `starter_not_allowed`
- [ ] 1.2 `starter_stage` 白名单与技能注入；非搭建会话返回 `starter_stage_invalid`
- [ ] 1.3 搭建会话视为可写，但委托令牌仍降权；站点只允许 `web/` 前缀

## 2. 前端分阶段驱动

- [ ] 2.1 新增 `useStarterRun`：上一阶段成功后自动发下一阶段，失败停住并提供重试 / 跳过 / 接手
- [ ] 2.2 手动发消息退出自动模式；刷新后按会话状态恢复进度
- [ ] 2.3 进度条与阶段文案补 zh-CN / en-US

## 3. 交付卡片

- [ ] 3.1 解析 `<!-- sb:delivery {...} -->`，新增 `StarterDeliveryCard.vue`
- [ ] 3.2 站点 URL 与后端同源校验；函数地址按 origin + 项目 ID 重拼
- [ ] 3.3 字段缺失逐项隐藏；整体缺失回退普通消息 + 资源入口

## 4. 提示词与文档

- [ ] 4.1 `gocloudagent/cloudagent/modules.go` 增加 `starterModulePrompt`（分阶段、只追加迁移、单文件前端、≤200KiB、无外链、不含 Key）
- [ ] 4.2 `docs/workbench/agent.md` 与实现对齐；未实现前不描述 `starter_stage`
- [ ] 4.3 用例：模板 / 阶段校验、自动推进、失败恢复、交付块解析与降级

## 5. 验证

- [ ] 5.1 `go test ./...`、`TestArchitectureGate`、前端 `typecheck` + `test` + `yarn build` 通过
- [ ] 5.2 端到端：从首页提示词到交付卡片的完整链路可在本地跑通
