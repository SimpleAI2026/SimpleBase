# Design

## 判定纪律

1. **代码是唯一事实**：不采信计划自述状态；每条先 `rg` 定位文件与符号，记录证据后再决定「做 / 已做 / 不做」。
2. **先关闭再新增**：已完成的条目直接勾选并记录证据，不重复实现；与现有 spec 冲突的以 spec 为准。
3. **行为变化单独立项**：涉及公开 API、错误码、数据模型、页面交互的条目，先补 delta spec 或拆独立 change；不得借「遗留清理」夹带行为变更。
4. **不可信验证不算通过**：没有跑过的命令保持「未验证」；禁止用日志过滤替代测试退出状态。

## 执行顺序

- 第一批（低风险、可独立回滚）：K 组工程债、J1/J2、D5 文档、I4 文档对齐。
- 第二批（有明确验收）：B3/B4、D2、E2/E3、G1/G2/G8。
- 第三批（需要环境）：A3/B5、C 组、D4、I2/I3、J7、K7。
- 第四批（需要产品决策）：D1/D3/D6、I1、J3–J6、G3–G7/G9/G10、H3/H4/H5。

## 与现有变更的关系

- 已拆出：`add-row-level-access-control`（H2）、`add-ops-maturity`（A1/A2/H1/H6）、`add-object-storage-site-delivery`（P1/P2/SITE-5）、`add-object-storage-path-access`、`add-project-mcp-server`、`add-api-preview-parameters`、`add-godatabase-runtime-refactor`、`complete-home-starter-delivery`、`complete-bi-dashboard`。
- 本变更只保留未拆出的条目；拆出后从本清单移除，避免双份跟踪。

## 停止条件

- 条目无法建立可信验证（缺环境 / 缺数据）→ 停在「待验证」，写明阻塞条件。
- 条目实施会改变未登记的外部行为 → 停止并补 spec 变更。
- 条目已被后续实现取代 → 直接关闭并记录替代位置。
