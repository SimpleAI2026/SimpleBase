# Mode table (full)

Step 0 of SKILL.md (build steps: B3 budget, B4 tokens, B5 execute, B6 hard checks, B7 polish) picks one primary mode plus any overlay. This is the full table with the mode-specific steps each reference file drives.

| User intent | Mode | Path | Load | Mode-specific steps |
|---|---|---|---|---|
| Build a new page/app/component | **Create** | Build | `references/direction.md`, `references/principles.md`, `references/tokens.md` | B3–B7 as written |
| "Review / audit / what's wrong with this UI" | **Audit** | Review | `references/audit.md` | `audit.md` §1 checks → §2 checklist → §3 report |
| "Redesign / upgrade / polish / make it premium" | **Redesign** | Review → Build | `references/audit.md` then `references/redesign.md` | `redesign.md` Workflow 1–9 (R3–R6 = its step 2; B3–B7 = its steps 4–9) |
| "Minimal / clean / editorial / Notion-like" | **Minimalist** (style overlay) | Same as primary mode | `references/minimalist.md` | Apply the spec in Step 2 (tone) and B4 (tokens); check it in B6 |
| Slides, docs, reports, HTML handouts needing a look | **Theme** | Build | `references/theming.md` | Theme recipe in `theming.md` drives B4–B5 |
| Poster, cover, static art (PNG/PDF) | **Artifact** | Build | `references/artifacts.md` | Philosophy → canvas process in `artifacts.md` drives B5; B6 word/color checks apply |
| "Review / score / beautify this website URL", benchmark vs. competitors (评审网站, 网站美化, 对标) | **Site Review** | Review | `references/site-review.md`, `references/benchmarks.md`, run `scripts/review_site.py` | `site-review.md` §1 capture → §2 score → §3 compare → §4 report (§5 batch) |
| "Rate / critique / what's wrong with this image", poster or cover review, AI-image critique (图片审美, 图片评审, 海报评审, AI 图片评价, 构图) | **Image Review** | Review | `references/image-aesthetics.md`, run `scripts/review_image.py <img> --type …` | R3 script metrics (§6) → R4 8 dimensions × type weights (§1–2) + AI checklist (§4) → R6 severity (§3) → report (§7) |
| Game screenshot / key art / Steam capsule / icon / HUD / concept / sprite review, genre art direction (游戏美术, 游戏审美, 游戏 UI, 游戏截图评审, RPG, MOBA, 二次元, 像素) | **Game Art Review** | Review | `references/game-aesthetics.md`, `references/game-workflows.md`, run `scripts/review_image.py <img> --type game-screenshot\|key-art\|capsule\|icon\|game-ui\|concept\|sprite --genre <g>` | `game-workflows.md` §1 (step by step) → §2 report; fixes in §3 |
| "Make this photo a magazine page / product hero / poster", extract a palette, image-model prompts, scientific figures (配色提取, 科研配图) | **Image Restyle** | Build | `references/image-workflows.md` (+ `image-aesthetics.md` to score before/after) | `image-workflows.md` §1 4-step pipeline = B4–B5; §3 restyle loop = B6–B7 |

Modes combine (e.g. Redesign + Minimalist). Read only the files you need.
