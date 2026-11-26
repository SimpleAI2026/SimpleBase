# Proposal

## Why

数据浏览页已重写为 BI 看板，R1/R2 的正确性问题大多修复，但 R3 视觉整理（BI-V01–V06）与 R4 增强（全屏、PNG/CSV 导出、配置导入导出、KPI 环比、时间缩放）尚未交付，真实浏览器验收也未执行。当前看板「能用」，但信息层级、截断口径与可分享产物仍不达标。

## What Changes

- R3 视觉整理：首屏信息层级与密度、KPI 与图表分层、图表可读性与数值表达、卡片操作与窄屏适配、筛选 / 编辑表单降噪、明细可读性与键盘可用性。
- R4 增强：单图 PNG / CSV 导出、图表全屏、看板配置导入导出（校验字段与类型、覆盖前确认）、KPI 环比（同长度上一时间窗）、时间桶切换与 DataZoom、卡片拖动排序。
- 导出与截断口径：PNG / CSV 必须标注当前筛选与是否截断，不得把部分分组当成全量。
- 真实浏览器验收：桌面与窄屏、浅色 / 深色、键盘操作与空数据场景。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `console-ui`: 增加 BI 看板的视觉层级、导出 / 全屏 / 配置迁移、KPI 环比与时间缩放契约。

## Impact

- 受影响代码：`ui/src/components/bi/*`、`ui/src/composables/{useBiDashboard,useBiDataSource,useBiDetail,useBiAgent}.ts`、`ui/src/utils/bi-*.ts`、`ui/src/components/databases/DataBrowser*`、`ui/src/locales/`、`ui/tests/`。
- 后端仅在必要时扩展只读聚合参数（环比复用现有聚合，不新增写接口）。
- 不改变：只读约束、聚合截断语义、看板本地持久化键、Agent 生成块进入看板前必须确认。
