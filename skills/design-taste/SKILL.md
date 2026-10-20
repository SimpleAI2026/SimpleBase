---
name: design-taste
description: Design-taste guide for making visual output look intentional, not templated "AI slop". Use to design, build, restyle, redesign, polish, critique or audit a UI (web page, landing page, dashboard, app screen, component), set up design tokens, pick colors/fonts/spacing, theme slides/docs, make a minimalist look, review/benchmark a live website URL, review or restyle images (photo, product, poster, banner, illustration, scientific figure, AI image — composition, palette, AI-artifact critique), review game art (screenshot, key art, Steam capsule, icon, HUD, concept, sprite/pixel; genre readability), or create a poster/cover. Chinese triggers include 设计界面, 美化页面, 界面评审, UI 审查, 重新设计, 改版, 设计规范, 配色, 字体搭配, 极简风格, 主题, 海报, 网站评审, 官网改版, 对标, 图片审美, 图片评审, 海报评审, 配色提取, 构图, 科研配图, AI 图片评价, 游戏美术, 游戏审美, 游戏 UI, RPG, MOBA, 二次元, 像素, 游戏截图评审. Not for backend logic or code cleanup with no visible change.
license: MIT
metadata:
  version: "1.1.0"
---

> This file is longer than one read (~4,000 characters). Keep reading in chunks to the end (review path, Constraints, build path, report, reference map) before you start.

# Design Taste

Taste here means **one clear direction, few well-chosen tokens, strong hierarchy, and no default-looking filler**. Every mode runs Steps 0–2, then the **review path (R3–R6)** or the **build path (B3–B7)**, then the **final report**. Step labels are for your tracking; don't paste them into the deliverable.

## Step 0 — Intake and mode
- **Do:** read the request and artifacts (code, URL, image). Pick one primary mode (+ overlay) below; the full table with mode-specific steps is `references/modes.md`.
- **Output:** mode, overlay, path, reference files to load.
- **Decision:** critique/score/benchmark only → review path; create or change something visible → build path; Redesign = review path as audit, then build path. No visible output (backend, cleanup) → this skill doesn't apply.

| Mode | Path | Load |
|---|---|---|
| Audit (UI/code review) | Review | `audit.md` |
| Site Review (URL, 对标) | Review | `site-review.md`, `benchmarks.md`, run `scripts/review_site.py` |
| Image Review (photo/poster/AI image) | Review | `image-aesthetics.md`, run `scripts/review_image.py <img> --type photo\|product\|poster\|social\|ui\|illustration\|figure\|ai` |
| Game Art Review | Review | `game-aesthetics.md`, `game-workflows.md`, run `review_image.py --type game-screenshot\|key-art\|capsule\|icon\|game-ui\|concept\|sprite --genre <g>` |
| Redesign | Review → Build | `audit.md`, then `redesign.md` |
| Create (page/app/component) | Build | `direction.md`, `principles.md`, `tokens.md` |
| Theme (slides/docs) | Build | `theming.md` |
| Artifact (poster/cover PNG/PDF) | Build | `artifacts.md` |
| Image Restyle / palette / figures | Build | `image-workflows.md` |
| Minimalist (overlay) | Same | `minimalist.md` |

## Step 1 — Gather context
- **Do:** find stack, CSS approach, existing tokens/design system, brand, constraints. Get baseline numbers: `review_site.py` for URLs, `review_image.py --type … [--genre …]` for images (`--lang zh` for Chinese users). If OCR (`tesseract`) is available, read the text in images with it.
- **Output:** short context note + baseline metrics (or "no tooling — visual judgment only").
- **Done when:** you know what exists and must be preserved. Missing artifact → ask for it.

## Step 2 — Decide a direction
- **Do:** write 3–5 lines before building or scoring: purpose & audience; **one tone** ("clean and modern" is not a tone); **one signature**; constraints; density. Review path: the direction *as perceived* is the yardstick.
- **Output:** the direction block (`direction.md` §5); for build tasks show it to the user before the first file write.
- **Done when:** it names one tone and one signature; an existing design system means "follow it well".

## Review path (Audit, Site Review, Image Review, Game Art Review; first half of Redesign)

### R3 — Measure
- **Do:** run the mode's measurable checks: `audit.md` §1 grep counts (distinct hex colors, font sizes, radii, shadows), `review_site.py`, or `review_image.py` (+ grayscale/thumbnail for games). Never modify the artifact under review.
- **Output:** metrics table with real counts.
- **Done when:** every metric the report template needs has a number or "not measurable: why".

### R4 — Score
- **Do:** score with the mode's rubric (`audit.md` §2, `site-review.md` §2, `image-aesthetics.md` 8 dims + AI checklist, `game-aesthetics.md` G1–G11); combine metric and visual scores as the reference says.
- **Output:** per-dimension scores with concrete evidence.
- **Done when:** every dimension has score + evidence; any P0 caps the total per the rubric.

### R5 — Compare (Site Review and Game Art Review only)
- **Do:** compare with `benchmarks.md`/`benchmarks.csv` or the genre profile. **Output:** gap table + 2–3 takeaways. **Done when:** each gap names the benchmark value.

### R6 — Prioritize
- **Do:** P0/P1/P2 issues, each with what, where (file:line / region), why, concrete fix (token values, HEX, crop, prompt).
- **Output:** prioritized list. **Done when:** every item has where/why/fix. Redesign → B3; else → final report.

## Constraints (checked in B6 and in reviews)

### Core rules (always on)
- **Commit.** A bold minimal design and a maximal one are both fine; a timid average of everything is not.
- **Tokens, not literals.** Every color, size, radius, shadow, and duration comes from a small named scale (see `references/tokens.md`).
- **Restraint in counts:** ~1 accent color, 1–2 typefaces, ≤ 6–8 font sizes, one radius family, ≤ 3 shadow levels, one easing curve family.
- **Hierarchy through contrast** of size, weight, and space — not more boxes, borders, and colors.
- **Spacing on a grid** (4/8px base). Group related things tightly, separate groups generously.
- **Accessibility is part of taste:** text contrast ≥ 4.5:1 (3:1 large), visible focus, hit targets ≥ 44px on touch, respect `prefers-reduced-motion`.
- **Real content.** No "John Doe / Acme / Lorem ipsum", no "Elevate your workflow" copy, no emoji as icons.
- **Every state exists:** hover, focus, active, disabled, loading, empty, error.

### AI-slop fingerprint (avoid unless the brief explicitly calls for it)
- Purple→blue gradients on white; neon glows on dark; gradient text everywhere.
- Inter/Roboto/Arial by reflex; one weight for everything.
- Centered hero + three identical icon cards + generic CTA band.
- Cards nested in cards; every element in a rounded bordered box with a drop shadow.
- Glassmorphism for no reason; random decorative blobs.
- Uniform `rounded-2xl` + `shadow-lg` on everything; inconsistent radii.
- Fake metrics ("10x faster", "99.9%") and stock testimonials.
- Bouncy/elastic animation on everything; animating layout properties.

Details and alternatives: `references/principles.md`.

## Build path (Create, Redesign, Theme, Artifact, Image Restyle)

### B3 — Budget and plan
- **Do:** plan so a **complete, valid first version exists by about 40% of your step/time budget** (roughly 15 tool calls or 8 minutes for a page or poster). Write the main file in 1–3 writes, not dozens of appends. Polish only after the complete version renders.
- **Output:** a section list (page), or for a poster the **exact copy written out with a word count** (≤ 25 words total, small print included; don't invent extra details such as prices, addresses, lineups or series numbers the user didn't give) plus 2–4 colors + neutrals. Fonts for either: at most 2 families in total — a monospace or label face counts as one.
- **Done when:** the plan fits the budget and the poster copy count is ≤ 25; anything that doesn't fit is cut now, not later.

### B4 — Tokens first
- **Do:** a small named scale for color, type, spacing, radius, shadow, motion as CSS variables / theme config (`tokens.md`); Minimalist: `minimalist.md` values.
- **Output:** token block. **Done when:** ≈1 accent, 1–2 typefaces, ≤ 6–8 sizes, one radius family, ≤ 3 shadows.

### B5 — Execute the complete version
- **Do:** build global layer → every planned section/element → states, using only tokens, in the user's stack; include `prefers-reduced-motion` and the closing tags in the first pass. Artifacts: render the PNG/PDF now.
- **Output:** the complete file(s) / rendered artifact.
- **Done when:** the whole scope exists and renders (HTML ends with `</html>`, the PDF opens). Then B6 — no polishing before this.

### B6 — Hard checks
- **Do:** walk the Constraints line by line, then measure: hex/font-size/radius counts; distinct font families (≤ 2, monospace included); contrast; screenshot at ~375/1280px. **Posters/artifacts:** list every text string on the canvas and count its words (letter-spaced or outlined text still counts; `pdftotext` misses outlined text, so count from your source file) — must be ≤ 25 unless the brief supplies more copy; over → delete lines, don't shrink them; count dominant colors in the render (≥ 2% of pixels, after quantizing) — must be ≤ 4 plus neutrals; fonts embedded or outlined; nothing clipped. Run existing tests/build; Redesign and Image Restyle re-run the Step 1 script and compare before/after numbers.
- **Output:** check results with numbers.
- **Decision:** any failure → fix in B5 and re-check; all pass → B7.

### B7 — Polish (only with budget left)
- **Do:** refine spacing, type, details; re-run B6 after changes. **Output:** improved files. **Done when:** budget is spent or nothing reads generic; never leave the file worse than the last passing version.

## Final step — Report
- **Do:** write in the user's language (`--lang zh` gives a Chinese report skeleton; replace every TODO). Build: the direction and trade-offs in 2–4 lines, what was made (file paths), B6 numbers. Review: the mode's template (`audit.md` §3, `site-review.md` §4, `image-aesthetics.md` §7, `game-workflows.md` §2), leading with a direct answer to the user's question.
- **Output:** the deliverable message (plus report files/screenshots).
- **Done when:** every claim is backed by a number or observation from R3/R4 or B6; nothing unverified is presented as checked.

## Reference map

- `references/modes.md` — full mode table with mode-specific steps per reference file.

- `references/direction.md` — direction prompts, tone catalog, how to pick fonts/palettes per tone.
- `references/principles.md` — color, typography, spacing, radius, shadow, motion, layout rules + anti-patterns.
- `references/tokens.md` — 3-tier token system (primitive → semantic → component) with example.
- `references/audit.md` — P0/P1/P2 checklist, measurable checks, report format.
- `references/redesign.md` — safe upgrade workflow for existing projects.
- `references/minimalist.md` — warm-monochrome editorial style spec.
- `references/theming.md` — themes for slides/docs/reports; theme recipe.
- `references/artifacts.md` — posters and static visual pieces (philosophy → canvas).
- `references/site-review.md` — URL review workflow: capture → metrics → 45/55 score → P0/P1/P2 report.
- `references/benchmarks.md` — top-company homepage breakdowns, industry playbooks, metric targets, common mistakes.
- `scripts/review_site.py` — Playwright capture + metric score + report skeleton; `scripts/benchmarks.csv` — 147 scored sites.
- `references/image-aesthetics.md` — image rubric: 8 dimensions × type weights, P0/P1/P2, AI-artifact checklist, metric ranges.
- `references/image-workflows.md` — parse → palette → layout → enhance pipeline; zine/collage/product/palette/cover recipes; Nature-style figure checklist; anti-slop prompt templates.
- `scripts/review_image.py` — Pillow+numpy image metrics (palette, harmony, tone, sharpness, noise, colorfulness, thirds/centroid, whitespace) → metrics.json, palette.png, thirds-overlay.png, report.md.
- `references/game-aesthetics.md` — game rubric: 11 dims (readability, value, gameplay clarity, HUD, VFX…) × asset weights, 21 genre profiles, 9 asset rubrics, AI checks, metric ranges.
- `references/game-workflows.md` — grayscale/silhouette/thumbnail/2-second tests, metric interpretation by genre, P0/P1/P2 report, fix recipes, anti-slop game-art prompts.
- `scripts/review_image.py --type <game type> --genre <g>` — adds value bands, figure-ground, shape clarity, HUD coverage, color count, store-thumbnail readability → grayscale.png, thumbnail.png.
- `references/sources.md` — upstream skills, URLs, and licenses.
