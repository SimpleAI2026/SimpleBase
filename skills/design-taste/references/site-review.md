# Site Review — score and beautify any URL

Use this when the user asks to review, benchmark or beautify a live website (评审网站 / 主页评审 / 网站美化 / 对标). The output is a Chinese report with a score, P0/P1/P2 issues and concrete token-level fixes.

## 1. Capture and measure

```bash
pip install playwright && python3 -m playwright install chromium   # one-time; nothing else is downloaded
python3 scripts/review_site.py https://example.com --out review-example --industry Software/Cloud --mobile
```

- The script uses headless Chromium. It loads the page, waits for network idle, tries to dismiss cookie banners (common button text plus OneTrust/TrustArc selectors) and scrolls to trigger lazy loading.
- It writes `fold.png` (1440×900), `full.png` (full page, at most 8000px), `mobile.png` (390×844, only with `--mobile`), `metrics.json` and the skeleton `report.md`.
- `--industry` should be one of the values in `scripts/benchmarks.csv`: Pharma, Finance, Internet, Software/Cloud, Semiconductors, Hardware/Devices, Retail, Luxury, Energy, Consumer Goods, Industrial, Aerospace.
- If the script warns that the page looks blocked (403, "Access Denied", "Just a moment"), say so in the report. Do not try to get around the bot protection. Ask the user for a screenshot, or review a local or staging build instead.
- Local or dev servers work too: pass `http://localhost:3000`.

Metrics are taken from computed styles of visible elements that carry text: distinct colour values (text/bg/border), font families, font sizes used at least twice, weights, radius and shadow values, gradients, hero headline size (the largest text in the first viewport), body size (character-weighted mode of text ≤20px), character-weighted WCAG contrast, share of text below AA, and hero media ratio.

### 1b. Key imagery (hero image)

- The script also saves the largest image/video/background element in the first viewport (≥ 12% of it) as `hero.png` and runs `review_image.py` on it into `hero-image/` (metrics.json, palette.png, thirds-overlay.png, report.md). Needs Pillow + numpy; skipped with a note otherwise. The hero image score is **reference only** and does not change the site metric score.
- Judge the hero with `image-aesthetics.md` (type `photo`, `product` or `illustration` as fits): subject clarity, does its palette share a source with the brand tokens, legibility of text laid over it, AI-artifact checklist for generated imagery, resolution vs. displayed size.
- Other key images (product tiles, team photos, OG/share image): screenshot or download them and run `python3 scripts/review_image.py <img> --type product|photo|social`. Report image problems in the same P0/P1/P2 list, tagged 【图片】.

Known noise to discount: carousels, product-tile pages (Apple-style zoning inflates colour counts), video that never rendered, and overlays that could not be dismissed.

## 2. Score

**Total = 0.45 × metric + 0.55 × visual**, which is the same formula as the benchmarks, so scores can be compared directly.

The metric score (computed by the script) is made of:
- colour 25%: −5 per colour above 12, −1.5 per gradient above 3
- type 25%: −10 per size tier above 7, −12 per family above 2, −8 per weight above 3
- consistency 15%: −8 per radius above 3, −10 per shadow above 3
- hierarchy 15%: 40 + 15 × min(hero/body, 4)
- accessibility 20%: contrast mapped to 30–100, −60 × share below AA, −15 if body <14px

The visual score is yours to give, 1–10 then ×10. Look at `fold.png`, then `full.png`, then `mobile.png`:
| Score | Meaning |
|---|---|
| 9 | Benchmark class (Lilly, Apple): one strong focus, signature type, flawless rhythm |
| 8 | Restrained and memorable, with a minor flaw (SpaceX, Inditex, JPMC) |
| 7 | Clean and competent, but generic or slightly busy |
| 5–6 | Template feel, crowded modules, weak hierarchy, overlay problems |
| 3–4 | Portal or promo wall, no focal point |
| ≤2 | Unstyled or broken |
Judge these: whitespace and spacing rhythm, hierarchy clarity, consistency, a signature element, and AI-slop or clutter tells (see SKILL.md Step 3). On mobile, check that nothing overflows, the hero still has one message, and tap targets are at least 44px.

## 3. Compare

- Fill in the metric table against the targets in `benchmarks.md` §2. The script pre-fills ✅/⚠️.
- Pick 1–2 close benchmarks from the same industry playbook (`benchmarks.md` §3) and name concrete gaps, e.g. "Hero 48px vs. IBM 54px / Robinhood 72px".
- Estimate the rank: `1 + count(benchmarks.csv score > total)`. Report it along with the industry average.

## 4. Report template (Chinese)

Start from the `report.md` the script wrote and replace every TODO:

```markdown
# 网站设计评审：<站点>
- URL / 行业 / 视口 / 截图 / 抓取时间（UTC+8）

## 1. 评分
指标分 X（45%）｜视觉分 Y（55%）｜**总分 Z**｜基准库约第 N/147 名，行业均分 M
子分：色彩/字号体系/一致性/层级/可访问性

## 2. 指标 vs 基准（Top 20 中位数）
（脚本生成的表格 + 1–2 句解读：哪些超标是"设计债"，哪些是测量噪音）

## 3. 对标
最接近的 1–2 个标杆，以及具体差距（数字）

## 4. 问题清单
### P0 — 可读性/可用性：低对比正文、弹层遮挡首屏、正文 <14px、移动端溢出、主 CTA 不可见
### P1 — 质感：颜色/字号/圆角/阴影超标、首屏多焦点、AI 模板感、门户化/促销化
### P2 — 打磨：间距节奏、字距/行高、图标风格不一、hover/focus 态缺失
每条写：现象（附数字或截图位置）→ 原因 → 修改方式

## 5. 美化建议
### Token 方案（CSS 变量；颜色 ≤12、字号 ≤7 档、圆角一族 ≤3、阴影 ≤2、渐变 0–1）
### Before → After 表（Hero / 导航 / 卡片 / CTA / 正文 / 页脚）
| 区域 | 现状 | 建议（具体值） |
### 预期收益：按改后的 token 数量重算指标分，作为估算

## 6. 一句话结论
```

Rules for the suggestions:
- Each suggestion must name a value: `48px → 64px/600, tracking −0.02em`, not "make it bigger".
- Keep the brand. Pull the accent from the site's existing brand colour and fix how it is used rather than replacing it.
- Order the fixes by return on effort: token consolidation and overlay fixes first, illustration or photo changes last.
- If the user wants code, continue in **Redesign** mode (`redesign.md`) and apply the token plan.

## 5. Batch / competitor mode

To compare a site with competitors, run the script once per URL, put the scores in a single table, and add one sentence per site. Do not crawl more than about 10 sites without asking the user first.
