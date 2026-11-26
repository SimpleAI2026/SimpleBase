# Spec Delta

## ADDED Requirements

### Requirement: 站点文件夹上传

控制台 SHALL 支持一次选择整个目录上传，按相对路径保留目录结构创建对象。每个文件 MUST 沿用单文件大小上限与 Content-Type 嗅探规则；任一文件失败 MUST 报告文件名与原因，已成功文件不回滚但需在结果中列出。

#### Scenario: 上传站点目录
- **WHEN** 用户选择包含 `index.html` 与 `assets/app.js` 的目录
- **THEN** 两个对象按相对路径写入，站点可直接访问

#### Scenario: 单个文件超限
- **WHEN** 目录中某个文件超过上限
- **THEN** 该文件被拒绝，其余文件继续上传，结果列出失败文件

### Requirement: SPA 回退

站点 SHALL 支持 `spa_fallback` 开关：开启后，未命中的无扩展名路径 MUST 返回 `index.html` 内容（200）；带扩展名的静态资源未命中 MUST 返回 404，不得回退为 HTML。

#### Scenario: 刷新前端子路由
- **WHEN** 用户直接访问 `/orders/42` 且站点开启 `spa_fallback`
- **THEN** 返回 `index.html`，由前端路由渲染

#### Scenario: 缺失静态资源
- **WHEN** 请求 `/assets/missing.js` 且文件不存在
- **THEN** 返回 404，不返回 `index.html`

### Requirement: 站点配额与下线

站点 SHALL 支持按项目配置出口用量上限与请求数上限，超限时返回明确错误并记录 `site_egress` 用量；管理员 MUST 能全局下线站点，下线后立即返回 404 并写审计。

#### Scenario: 出口超限
- **WHEN** 项目站点出口用量达到配置上限
- **THEN** 后续请求返回配额错误，控制台展示用量与重置时间

#### Scenario: 管理员下线
- **WHEN** 管理员下线一个站点
- **THEN** 站点请求立即 404，审计记录操作者、项目与时间
