# Image Aesthetics — rubric for judging any image

Use for photos, product shots, posters/covers, social banners, UI screenshots, illustrations, scientific figures, and AI-generated images. Run `scripts/review_image.py <img> --type <t>` first for the numbers, then **look at the image** and score the dimensions below. Numbers catch blur, clipping and muddy palettes; only your eyes catch meaning, emotion and AI artifacts.

**Final score = 0.4 × metric score (script) + 0.6 × visual score (this rubric).** Visual score = Σ(dimension 1–10 × weight) / Σweights × 10.

## 1. Dimensions (score each 1–10)

| # | Dimension | What 9–10 looks like | What 3–4 looks like |
|---|---|---|---|
| D1 | **Composition & subject** | One unmistakable subject; placed on a thirds/golden point or deliberately centered with symmetry that means something; leading lines (roads, shorelines, arms, gaze) point into the subject; edges clean (no limbs/poles cut at joints, no tangents); negative space gives the subject air and has a shape of its own | Several things compete; subject dead-center by default; horizon tilted or cutting the frame in half; clutter at the edges; subject too small to read on a phone |
| D2 | **Light** | Direction and quality are intentional (side light for texture, back light for rim, soft window light for skin); highlights hold detail; shadows have shape; one consistent light source in composites | Flat on-camera flash; blown sky; muddy shadows; mixed color temperatures; in composites, light coming from two sides |
| D3 | **Color harmony & palette** | 1 dominant + 1–2 supporting + ≤1 accent (≤ 3–5 meaningful hues); a recognisable harmony (monochrome, analogous, complementary, split-complementary); skin tones natural; whites neutral unless graded on purpose | Rainbow of unrelated hues; neon saturation; color cast fighting the mood; accent color not taken from the scene |
| D4 | **Typography on image** (n/a if no text) | Text sits in calm areas (negative space, gradient scrim, solid block); ≥ 4.5:1 contrast against what is *actually behind it*; 1–2 typefaces, clear size hierarchy; aligned to an edge or grid; safe margins ≥ 5% of the short side | Text over busy detail; drop-shadow/outline to rescue legibility; 3+ fonts; centered everything; text touching edges; letter-spacing tight on display type |
| D5 | **Technical quality** (focus, sharpness, noise) | Critical focus on the subject's key point (eyes, product logo, data); noise appropriate to the medium; no compression blocks, banding, halos from over-sharpening; resolution fits the output (≥ 2× for retina, ≥ 300 dpi at print size) | Missed focus; motion blur by accident; heavy denoise wax; upscaling mush; JPEG blocks in gradients |
| D6 | **Style consistency** | Every element shares one look: grain, lens, grade, illustration stroke, corner style; series images share a color card | Photoreal subject pasted on a flat vector background; three grading styles in one carousel; mixed icon families |
| D7 | **Emotion & story** | The image says one thing; a viewer can name the feeling in a word; there is a moment, a tension or a question | Pleasant but forgettable; stock-photo smile; "concept" that needs a caption to understand |
| D8 | **AI-artifact hygiene** (AI-generated or suspected) | Nothing below is present | Any P0 artifact visible at normal viewing size |

## 2. Weights by image type

| Type (`--type`) | D1 Comp | D2 Light | D3 Color | D4 Type | D5 Tech | D6 Style | D7 Story | D8 AI | Specific must-haves |
|---|---|---|---|---|---|---|---|---|---|
| photo | 3 | 3 | 2 | 0 | 2 | 1 | 3 | 1 | straight horizon, focus on eyes/subject, no clipped highlights on skin |
| product | 2 | 3 | 2 | 1 | 3 | 2 | 1 | 2 | true product color, clean edges/cutout, shadow grounds the object, label readable, ≥ 30% clean background for marketplaces |
| poster / cover | 3 | 1 | 3 | 3 | 1 | 2 | 2 | 1 | readable title at thumbnail size (≈ 200px wide), one focal point, palette ≤ 4 |
| social banner / thumbnail | 2 | 1 | 2 | 3 | 1 | 1 | 2 | 1 | works at 25% size; key text inside safe area; face/subject large; ≤ 7 words |
| UI screenshot | 2 | 0 | 2 | 3 | 2 | 3 | 0 | 0 | judge with `audit.md`; here check hierarchy, density, whitespace ratio, color count |
| illustration | 3 | 1 | 3 | 1 | 1 | 3 | 3 | 1 | consistent stroke and lighting logic, limited palette, readable silhouettes |
| scientific figure | 2 | 0 | 2 | 3 | 2 | 2 | 2 | 0 | conclusion-first, colorblind-safe, panel labels, 5–7 pt text at final size (see §5) |
| AI-generated | 2 | 2 | 2 | 1 | 2 | 2 | 2 | 4 | run the full §4 checklist; downgrade to ≤ 5 overall if any P0 artifact |

Weight 0 = not applicable; leave it out of the average.

## 3. Severity

- **P0 — breaks the image**: subject out of focus; unreadable or misspelled text; AI hand/face/text artifacts visible at normal size; wrong product color; scientific figure misleading (truncated axis without note, red/green only encoding); crop cutting the subject's head/product.
- **P1 — clearly cheapens it**: cluttered composition; 6+ unrelated hues or neon over-saturation; blown highlights / crushed shadows over important areas; text on busy background; mixed styles; heavy HDR halos; low resolution for the output size.
- **P2 — polish**: slight tilt; minor color cast; spacing between title and subtitle; grain/sharpening fine-tuning; small tangents at edges.

## 4. AI-generated image checklist (P0 unless noted)

- **Hands & anatomy**: finger count, fused/bent fingers, extra limbs, teeth rows, asymmetrical eyes/pupils, earrings that don't match, hair merging into clothing.
- **Text gibberish**: pseudo-letters on signs, labels, book spines, screens; brand names misspelled. Fix by generating text-free and setting real type afterwards.
- **Plastic skin**: poreless wax, uniform specular sheen, airbrushed jawlines (P1).
- **Over-saturation & HDR glow**: teal-orange by reflex, neon rim light on everything, bloom around every highlight (P1). Script flag: saturation p95 > 0.92.
- **Symmetric slop**: dead-center subject, mirrored background, identical left/right elements (P2–P1). Script hint: `mirror_symmetry` > 0.85.
- **Physics & logic**: shadows pointing two ways, reflections that don't match, objects merging, impossible architecture, background people melting (P0–P1).
- **Texture repetition**: tiled bricks/leaves/crowd faces repeated (P1).
- **Generic "epic" look**: volumetric god-rays, lens flare, fog, orange-teal, centered hero pose — the image could illustrate anything (P1, story).
- **Edge/cutout halos** in composites and product swaps (P1).

## 5. Scientific figure quick check (Nature-style)

Conclusion stated before plotting · one hero panel · lowercase bold panel labels (a, b, c) top-left · sans-serif (Arial/Helvetica) 5–7 pt at final print size · colorblind-safe palette (Okabe-Ito or viridis/cividis), never red/green as the only cue, no rainbow/jet · same method = same color across panels · direct labels over legends · error bars/statistics defined · axis units · vector export (PDF/SVG/EPS) with embedded fonts (Type 42) · no decorative icons. Detailed list: `image-workflows.md` §4.

## 6. Reading the script metrics

| Metric | Healthy range (photo) | Notes |
|---|---|---|
| `sharpness_laplacian_var` | > 100 (analysis at ≤ 640px) | < 60 on a photo → likely blur. Flat graphics are naturally low. |
| `noise_sigma` | < 3 clean, 3–6 visible grain, > 8 noisy | measured on a full-res center crop; texture inflates it |
| `tone.clip_light_pct / clip_dark_pct` | < 3% / < 5% for photos | UI and posters often legitimately have flat white/black |
| `tone.contrast_rms` | 40–85 | < 30 flat/foggy, > 90 harsh |
| `tone.saturation_p95` | < 0.85 | > 0.92 = neon/over-graded risk |
| `colorfulness` (Hasler–Süsstrunk) | 15–33 "moderately", 33–59 "quite", > 82 "extremely" colorful | describe, don't judge, by itself |
| `palette_significant_colors` (≥ 3%) | ≤ 5 poster/product, ≤ 8 photo | merged k-means clusters; neutrals count |
| `harmony_guess` | — | guess from hue clusters; confirm visually |
| `composition.centroid_to_thirds_point` | < 0.12 for thirds-style images | edge-energy centroid ≈ visual weight; small distance to center can be intentional |
| `composition.mirror_symmetry` | — | > 0.85 = near-mirror symmetry |
| `whitespace_ratio` | poster 0.25–0.75, product 0.3–0.8, figure 0.4–0.85 | share of flat 16px blocks |
| `text_region_ratio_est` | — | rough stroke-pattern proxy; verify by eye |

## 7. Report format

Use the `report.md` skeleton the script writes (`--lang en|zh`): score → key metrics → visual review (per-dimension score + evidence) → P0/P1/P2 → actionable fixes (crop ratio, grading values, HEX, layout, regeneration prompt) → one-line verdict. Always cite what you see ("six fingers on the lower-left hand", "title sits on leaf texture, contrast ≈ 2:1"), never just adjectives.
