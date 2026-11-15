# Skills 优化执行记录 — M0–M2 轮次

执行日期：2026-11-15。依据：`skills-optimization-plan.md` §7–§9 的 M0/M1/M2 任务卡。
范围：本轮只做可离线验证的 P0 修复与基础设施；M3 行为评测（需要模型运行）、M4 runner、M5 描述瘦身、M6 发布审查未做。

## 执行摘要

| 计划任务 | 状态 | 修改文件 | 证据 |
|---|---|---|---|
| M0 基线冻结 | 完成 | 无改动 | `.session_tmps/…/M0-baseline.txt`（7 技能指纹） |
| SB-01 凭据绑定 | 完成 | `simplebase/scripts/sbdev.py` | 新增测试 fail-closed 断言通过 |
| SB-03 错误脱敏 | 完成 | 同上 | `redact()` + CLI 出口测试通过 |
| SB-04 测试入口 | 完成 | `simplebase/scripts/test_sbdev.py` | 直接执行 34 = discover 34（原 25≠31） |
| SKILL.md 凭据描述同步 | 完成 | `simplebase/SKILL.md:24` | 与实现一致 |
| AT-01/02/03 隔离+manifest+显式拼接 | 完成 | `andrew-translate/SKILL.md` §2 | oracle 模拟：旧块污染 0 |
| DT-01 no-clobber | 完成 | `design-taste/scripts/review_image.py`、`review_site.py` | 哨兵默认保留（exit 1），`--force` 才覆盖 |
| DT-03 consent 默认不点 | 完成 | `review_site.py` | `--accept-consent` 显式开关 |
| CR-04 TS 参考 | 完成 | `code-refactor/evals/files/08-ts-to-go-port/compare.py` | bun 参考 3/0；坏参考 exit 2 |
| CR-05 崩溃标注 | 完成 | 两个 compare.py | false 命令显式 `<command failed exit=1>` |
| AL-01 strict 校验 | 完成 | `ambition-loop/scripts/check_report.py`（+zh 同步） | 伪报告 FAIL、合法报告 PASS、zh sync 通过 |
| TS-01 删除授权 | 完成 | `tester-skill/subskills/test-audit/SKILL.md` | 建议清单/已授权删除分离；mutation 隔离边界 |
| INF-01 校验器 | 完成 | `tools/validate_skills.py`（新增） | 7 技能 0 error、4 warning（描述长度，M5 处理） |
| AT/DT evals 补断言 | 完成 | `andrew-translate/evals/evals.json`（+41）、`design-taste/evals/evals.json`（+27） | 校验器从 17 error → 0 |
| INF-06 卫生 | 完成 | 删 7 技能内全部 `__pycache__` | find 计数 0 |

## 最终回归（全部通过）

1. simplebase：discover 34 tests OK；直接执行 34 tests OK；Go `internal/sbcli` ok（双实现 parity）。
2. ambition-loop：good-report `--strict` PASS；fake-report `--strict` FAIL；`check_sync.py` in sync（11 steps / 8 evals / 20 files）。
3. code-refactor compare：04（Python 参考）4/0；08（bun 参考）3/0。
4. simplify-code fixtures：Python 4 例 + JS 5 例全 PASS（与修复前基线一致）。
5. design-taste：合成图审查 exit 0；默认 no-clobber exit 1。
6. `tools/validate_skills.py`：7 dirs / 0 errors / 4 warnings。

## 修改后指纹（回滚锚点）

ambition-loop 15df85d95b038377 · andrew-translate 56df08eca614eeba · code-refactor b63076dd19320b41 · design-taste e51ccd19675efe99 · simplebase 9a186762f7629956 · simplify-code 31f054c19e1f5906 · tester-skill ddc7e87ad911bbd6

修改前基线见 `.session_tmps/…/M0-baseline.txt`。`simplify-code` 指纹变化仅因删除 `__pycache__`（内容无改动，已用其 evals fixture 全 PASS 复核）。

## 行为变更说明（用户可见）

- `sbdev.py --env-file` 缺 Key：原静默回退进程 Key（可致云端 URL 配本地 Key），现直接报错。
- CLI 错误消息：已知密钥样式（sb_live_/sk-/Bearer/token=…）上抛前掩码为 `***`。
- `review_image.py` / `review_site.py`：输出目录非空时默认拒绝（exit 1），须 `--force`；网站审查默认不再点任何 consent 按钮，须 `--accept-consent`。
- `check_report.py` 新增 `--strict`（CI 用）；不带参数的旧行为完全不变。
- compare.py（08）默认参考从不可运行的伪命令改为 bun/tsx 自动探测 + 健全性自检。

## 未做（后续里程碑）

- M3：行为评测运行（需模型）+ 触发样本扩容；`tools/validate_skills.py` 已就绪。
- M4：runner + 安全日志 + baseline/with-skill 各 3 次。
- M5：四个 >700 描述瘦身（校验器已标 WARN）；触发消歧需 M3 数据。
- SB-05/06：simplebase 12+ 行为 evals 与 Go 端文案同步项（新增文案时同提交）。
- CR-02/03：rewrite harness 补差分语料（需模型参与验证）。

## 回滚

单技能回滚：`cd skills/<name> && git checkout -- <files>`（simplebase/design-taste/tester-skill 在主仓库；ambition-loop/andrew-translate/code-refactor/simplify-code 是子模块）。全量回滚按上表指纹核对。
