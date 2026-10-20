# Image Workflows — aesthetic engineering & restyle recipes

For "make this photo look like a magazine page / product hero / poster", palette extraction, and prompts for image models. Pair with `image-aesthetics.md` to score before and after.

## 1. The 4-step pipeline

1. **Parse** — before touching pixels, write 3 lines: *subject* (what must stay recognisable — face, product, building silhouette, data), *space* (foreground/background, horizon, empty areas), *mood* (one word). Decide **keep the photo** (photo is the anchor) or **distill it** (photo is only inspiration; output is a new illustration).
2. **Extract palette from the source** — run `review_image.py` and take `palette`. Pick: 1 dominant (often the largest neutral), 1 support, and **one accent that already exists in the scene** (a sign, a coat, sky). Never import an accent with no relationship to the photo, and don't let a strong object color (red car, sunset) automatically become the whole layout's accent — compare 2–3 candidates. For a series, build one shared color card (paper tone, type color, accent) and reuse it.
3. **Layout & whitespace rules** — choose the canvas first (3:5 or 4:5 portrait for zines/social, 2:3 for covers, 1:1 for marketplaces, 16:9 / 1.91:1 for banners). Then: one main composition, one container for the subject, one material (+ at most one secondary), one type pairing with clear role contrast, one color source, zero or one decoration — and only a decoration that does a job. Margins ≥ 6% of the short side; for minimal layouts let empty paper take 60–90% and the subject 8–25%.
4. **Enhance / restyle** — order: crop & straighten → exposure/white balance → local contrast → color grade toward the palette → retouch (dust, distractions) → upscale only if needed (e.g. Real-ESRGAN, BSD-3) → typography last. Re-run `review_image.py` and compare metric deltas; then look again.

## 2. Recipes

| Recipe | Use when | Canvas & layout | Color | Type | Don't |
|---|---|---|---|---|---|
| **Minimal magazine / zine** | a calm travel/still-life photo or a single line of text | 3:5 or 4:5, aged off-white paper, subject small (8–25%), 70–90% empty, aligned to a corner or thirds point | near-monochrome + **one** saturated accent from the image | small serif or typewriter caption, archive-style metadata (place · date · No.) | add frames, stickers, gradients; enlarge the subject to fill |
| **Paper collage (keep-photo)** | street/people photos where the real place matters | photo window as anchor; torn-fiber edges; abstract shapes derived from the photo's lines extend onto paper | separate "colors in the photo" from "new print color"; one adaptive spot color | short title + tiny captions | let the photo's red object leak into every shape; cover faces or key relations |
| **Photo + abstract panel (editorial)** | architecture, skylines, water, roads | top: untouched photo; bottom: ivory panel with 3–5 flat geometric marks abstracted from the photo's spatial relations | 2–3 colors sampled from photo | 2–5 word title | textures, grain, gradients in the panel |
| **Product hero shot** | e-commerce, launch page, ad | subject 40–60% of frame, slightly below center or on a thirds line; seamless background; contact shadow + soft reflection to ground it | background derived from product's secondary color or a neutral; true-to-life product color (check against reference) | product name + one benefit, outside the product silhouette | floating object without shadow; props that steal focus; colored rim light that shifts product color |
| **Palette recomposition** | brand a set of photos, or derive a site/poster theme from one image | — | k-means palette → choose dominant/support/accent → map to tokens (`--bg`, `--fg`, `--accent`, `--muted`) → check text contrast ≥ 4.5:1 → grade all images toward the card | — | pick the most saturated cluster as the background |
| **Editorial cover** | magazine/report/book/podcast cover | 2:3 or 1:1; masthead zone fixed; one image, one cover line hierarchy (title ≫ subtitle ≫ meta); subject can overlap the masthead once, deliberately | 2 colors + photo | display face with character for title, neutral sans for meta | five cover lines, bursts, outlines; title over a face |

## 3. Restyle loop

Score → pick the 1–3 biggest problems (P0 first) → apply one recipe → re-score. Report the metric deltas (e.g. "significant colors 9 → 4, saturation p95 0.97 → 0.82, centroid-to-thirds 0.21 → 0.06") and show before/after side by side.

## 4. Scientific figure (Nature-style) checklist

1. **Contract first**: one-sentence conclusion; which panel proves it (hero panel); evidence chain a → b → c.
2. **Tools**: Matplotlib/Seaborn (Python) or ggplot2/patchwork (R) — pick one backend and stay in it.
3. **Size**: build at final size (single column ≈ 89 mm, double ≈ 183 mm); text 5–7 pt at that size; lines ≥ 0.5 pt.
4. **Type**: Arial/Helvetica, sentence case, no colored text; panel labels lowercase bold **a, b, c** at top-left of each axes, same offset everywhere.
5. **Color**: colorblind-safe (Okabe-Ito categorical; viridis/cividis sequential; diverging with neutral midpoint); never encode only by red vs green; no rainbow/jet; one method = one hue family across all panels; grey for baselines, accent for "ours".
6. **Honesty**: axes start at zero for bars; show n, error bar definition and test; don't crop gel/blot images selectively; keep source data.
7. **Layout**: equal panel widths in a row, aligned axes edges, consistent gutters; direct labels > legends; remove chart junk (box frames, gridlines unless needed, 3-D, shadows, icons).
8. **Export**: vector PDF/SVG/EPS, fonts embedded (pdf.fonttype 42), raster parts ≥ 300 dpi (line art 600–1200).
9. **QA**: view at 100% print size and at 50%; grayscale test; colorblind simulation; check overlaps and clipping.

## 5. Prompt templates that avoid AI-slop

Structure every prompt as **subject → composition → light → palette → medium/lens → texture → negative constraints**. Be concrete; ban the defaults.

**Photo-real**
```
[subject + action, specific age/material/brand-neutral], [shot type: 35mm eye-level medium shot / 85mm close-up / top-down],
subject on the left third, horizon on the lower third, generous negative space on the right,
[light: soft overcast window light from camera-left / low golden side light / single hard key with deep shadow],
palette limited to [#hex dominant], [#hex support], small accent of [#hex] from [object],
natural skin texture with pores, realistic imperfections, slight film grain, true-to-life color, no HDR,
avoid: centered symmetric composition, glowing rim light, teal-orange grade, bokeh balls, lens flare, text, watermark, extra fingers
```

**Product hero**
```
[product] on a seamless [color] sweep, three-quarter view, product occupies 50% of frame slightly below center,
large softbox key from top-left, white bounce fill, soft contact shadow and faint reflection,
exact product color [#hex], crisp label edges, no props, no floating, no text, no glow, no gradient background
```

**Minimal zine poster**
```
3:5 vertical aged off-white paper, 80% empty space, a single small [subject] placed near the lower-left thirds point,
one saturated accent color [#hex taken from source photo], everything else muted warm greys,
risograph texture, quiet editorial layout; leave the top-right area empty for type (type added later),
avoid: frames, stickers, gradients, multiple accents, centered subject, generated text
```

**Scientific schematic / graphical abstract**
```
clean flat vector schematic of [mechanism], left-to-right flow in [3] stages, white background,
Okabe-Ito palette, thin uniform strokes, no 3-D, no shadows, no glow, no decorative icons, labels as blank boxes (text added in vector editor later)
```

Rules: generate **without text** and typeset afterwards; ask for **2–4 variants** that differ in composition, not just seed; specify the aspect ratio; reuse the exact hex palette across a series; after generation run the §4 AI checklist in `image-aesthetics.md` before delivering.
