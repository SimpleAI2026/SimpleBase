---
name: design-taste
description: Design-taste guide for making visual output look intentional, not templated "AI slop". Use to design, build, restyle, redesign, polish, critique or audit a UI (web page, landing page, dashboard, app screen, component), set up design tokens, pick colors/fonts/spacing, theme slides/docs, make a minimalist look, review/benchmark a live website URL, review or restyle images (photo, product, poster, banner, illustration, scientific figure, AI image — composition, palette, AI-artifact critique), review game art (screenshot, key art, Steam capsule, icon, HUD, concept, sprite/pixel; genre readability), or create a poster/cover. Chinese triggers include 设计界面, 美化页面, 界面评审, UI 审查, 重新设计, 改版, 设计规范, 配色, 字体搭配, 极简风格, 主题, 海报, 网站评审, 官网改版, 对标, 图片审美, 图片评审, 海报评审, 配色提取, 构图, 科研配图, AI 图片评价, 游戏美术, 游戏审美, 游戏 UI, RPG, MOBA, 二次元, 像素, 游戏截图评审. Not for backend logic or code cleanup with no visible change.
license: MIT
---

# Design Taste

Make deliberate visual decisions, then execute them consistently. Taste here means: **one clear direction, few well-chosen tokens, strong hierarchy, and no default-looking filler.**

## Step 0 — Pick the mode

| User intent | Mode | Load |
|---|---|---|
| Build a new page/app/component | **Create** | `references/direction.md`, `references/principles.md`, `references/tokens.md` |
| "Review / audit / what's wrong with this UI" | **Audit** | `references/audit.md` |
| "Redesign / upgrade / polish / make it premium" | **Redesign** | `references/audit.md` then `references/redesign.md` |
| "Minimal / clean / editorial / Notion-like" | **Minimalist** (style overlay) | `references/minimalist.md` |
| Slides, docs, reports, HTML handouts needing a look | **Theme** | `references/theming.md` |
| Poster, cover, static art (PNG/PDF) | **Artifact** | `references/artifacts.md` |
| "Review / score / beautify this website URL", benchmark vs. competitors (评审网站, 网站美化, 对标) | **Site Review** | `references/site-review.md`, `references/benchmarks.md`, run `scripts/review_site.py` |

| "Rate / critique / what's wrong with this image", poster or cover review, AI-image critique (图片审美, 图片评审, 海报评审, AI 图片评价, 构图) | **Image Review** | `references/image-aesthetics.md`, run `scripts/review_image.py <img> --type …` |
| Game screenshot / key art / Steam capsule / icon / HUD / concept / sprite review, genre art direction (游戏美术, 游戏审美, 游戏 UI, 游戏截图评审, RPG, MOBA, 二次元, 像素) | **Game Art Review** | `references/game-aesthetics.md`, `references/game-workflows.md`, run `scripts/review_image.py <img> --type game-screenshot\|key-art\|capsule\|icon\|game-ui\|concept\|sprite --genre <g>` |
| "Make this photo a magazine page / product hero / poster", extract a palette, image-model prompts, scientific figures (配色提取, 科研配图) | **Image Restyle** | `references/image-workflows.md` (+ `image-aesthetics.md` to score before/after) |

Modes combine (e.g. Redesign + Minimalist). Read only the files you need.

## Step 1 — Decide a direction before touching code

Write 3–5 lines (to yourself or the user) before building:

1. **Purpose & audience** — what job does this screen do, for whom?
2. **Tone** — choose one specific stance (e.g. editorial, brutalist, soft/organic, technical/dense, luxury/quiet, playful, retro-industrial). "Clean and modern" is not a direction.
3. **Signature** — the one thing someone will remember (a type pairing, a bold color field, an unusual grid, a motion moment).
4. **Constraints** — existing brand, framework, accessibility level, performance budget.
5. **Density** — airy marketing vs. dense tool. Dashboards ≠ landing pages.

If the project already has a design system, the direction is *"follow it well"*; don't invent a second one.

## Step 2 — Core rules (always on)

- **Commit.** A bold minimal design and a maximal one are both fine; a timid average of everything is not.
- **Tokens, not literals.** Every color, size, radius, shadow, and duration comes from a small named scale (see `references/tokens.md`).
- **Restraint in counts:** ~1 accent color, 1–2 typefaces, ≤ 6–8 font sizes, one radius family, ≤ 3 shadow levels, one easing curve family.
- **Hierarchy through contrast** of size, weight, and space — not more boxes, borders, and colors.
- **Spacing on a grid** (4/8px base). Group related things tightly, separate groups generously.
- **Accessibility is part of taste:** text contrast ≥ 4.5:1 (3:1 large), visible focus, hit targets ≥ 44px on touch, respect `prefers-reduced-motion`.
- **Real content.** No "John Doe / Acme / Lorem ipsum", no "Elevate your workflow" copy, no emoji as icons.
- **Every state exists:** hover, focus, active, disabled, loading, empty, error.

## Step 3 — Avoid the AI-slop fingerprint

Default tells to avoid unless the brief explicitly calls for them:

- Purple→blue gradients on white; neon glows on dark; gradient text everywhere.
- Inter/Roboto/Arial by reflex; one weight for everything.
- Centered hero + three identical icon cards + generic CTA band.
- Cards nested in cards; every element in a rounded bordered box with a drop shadow.
- Glassmorphism for no reason; random decorative blobs.
- Uniform `rounded-2xl` + `shadow-lg` on everything; inconsistent radii.
- Fake metrics ("10x faster", "99.9%") and stock testimonials.
- Bouncy/elastic animation on everything; animating layout properties.

Details and alternatives: `references/principles.md`.

## Step 4 — Execute

- Implement real, working code in the user's stack; match existing conventions.
- Define tokens first (CSS variables / theme config), then build components from them.
- Match effort to direction: maximal designs need elaborate execution; minimal designs need precision in spacing and type.

## Step 5 — Verify before handing off

Run the quick checks from `references/audit.md` (counts of hex colors, font sizes, radii; contrast; states; responsive at ~375/768/1280px). If you can render, take a screenshot and look at it — fix what reads as generic or cluttered. Report the direction you chose and any trade-offs in 2–4 lines.

## Reference map

- `references/direction.md` — direction prompts, tone catalog, how to pick fonts/palettes per tone.
- `references/principles.md` — color, typography, spacing, radius, shadow, motion, layout rules + anti-patterns.
- `references/tokens.md` — 3-tier token system (primitive → semantic → component) with example.
- `references/audit.md` — P0/P1/P2 checklist, measurable checks, report format.
- `references/redesign.md` — safe upgrade workflow for existing projects.
- `references/minimalist.md` — warm-monochrome editorial style spec.
- `references/theming.md` — themes for slides/docs/reports; theme recipe.
- `references/artifacts.md` — posters and static visual pieces (philosophy → canvas).
- `references/site-review.md` — URL review workflow: capture → metrics → 45/55 score → Chinese P0/P1/P2 report.
- `references/benchmarks.md` — top-company homepage breakdowns, industry playbooks, metric targets, common mistakes.
- `scripts/review_site.py` — Playwright capture + metric score + report skeleton; `scripts/benchmarks.csv` — 147 scored sites.
- `references/image-aesthetics.md` — image rubric: 8 dimensions × type weights, P0/P1/P2, AI-artifact checklist, metric ranges.
- `references/image-workflows.md` — parse → palette → layout → enhance pipeline; zine/collage/product/palette/cover recipes; Nature-style figure checklist; anti-slop prompt templates.
- `scripts/review_image.py` — Pillow+numpy image metrics (palette, harmony, tone, sharpness, noise, colorfulness, thirds/centroid, whitespace) → metrics.json, palette.png, thirds-overlay.png, report.md.
- `references/game-aesthetics.md` — game rubric: 11 dims (readability, value, gameplay clarity, HUD, VFX…) × asset weights, 21 genre profiles, 9 asset rubrics, AI checks, metric ranges.
- `references/game-workflows.md` — grayscale/silhouette/thumbnail/2-second tests, metric interpretation by genre, Chinese P0/P1/P2 report, fix recipes, anti-slop game-art prompts.
- `scripts/review_image.py --type <game type> --genre <g>` — adds value bands, figure-ground, shape clarity, HUD coverage, color count, store-thumbnail readability → grayscale.png, thumbnail.png.
- `references/sources.md` — upstream skills, URLs, and licenses.
