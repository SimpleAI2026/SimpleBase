# Game Workflows — review, report, fix, generate

## 1. Review a game image (step by step)

1. **Classify**: asset type (`game-screenshot | key-art | capsule | icon | game-ui | concept | sprite`) and genre (`python3 scripts/review_image.py --list-genres`). Ask or infer target platform (Steam, App Store, Google Play, console, in-game).
2. **Run metrics**
   ```bash
   python3 scripts/review_image.py shot.png --type game-screenshot --genre moba --out review-shot
   ```
   Outputs `metrics.json`, `report.md` (English skeleton by default; `--lang zh` for Chinese), `palette.png`, `thirds-overlay.png`, `grayscale.png`, `thumbnail.png`.
3. **Look** at the original, then `grayscale.png` (gray | 3-value posterize | Otsu figure-ground mask) and `thumbnail.png` (store sizes, color + gray on Steam navy).
4. **Run the five tests** and write evidence:
   | Test | How | Pass |
   |---|---|---|
   | Grayscale test | `grayscale.png` panel 1 | Player, enemies, interactables, logo still separable |
   | 3-value test | panel 2 | Focal area sits in its own value group; background compressed |
   | Silhouette test | panel 3 / mentally fill subjects black | Roles recognisable from outline; groups don't merge |
   | Thumbnail test | `thumbnail.png` at 184×69 / 120×45 / 128 / 64 | Title readable, genre guessable, one subject |
   | Gameplay 2-second test | Look for 2 s, look away | "Where am I, what threatens me, what can I interact with" answered |
5. **Interpret metrics by genre** (targets in `game-aesthetics.md §3/§6`):
   - Low `contrast_rms` / `figure_ground_gap` → readability risk; matters most for shooter, MOBA, BR, platformer; tolerated for horror/ink if focal accents exist.
   - `value_bands` one band > 85% → monotone; horror and night scenes may be dark-heavy but need a lit focal path.
   - `shape_edge_retention` < 0.30 with high `edge_density` → micro-detail noise, weak big shapes (common in AI art and over-detailed concept).
   - `hud_edge_coverage_est` vs genre max: SLG/MMO/idle/card tolerate 40–45%; shooter/open world ≤ 15%. `hud_center_clutter_est` > 0.15 = UI covering play space. (Heuristic — architecture and grids inflate it.)
   - Color count: stylised/casual want few bins; pixel art `unique_colors_exact` ≤ 256 and `pixel_run_mode` = consistent integer scale.
   - Thumbnail `contrast_rms` < 35 or retention < 0.75 on capsule/icon/key art → P0.
6. **Score** 11 dims with type weights (`game-aesthetics.md §2`); total = 0.4 metric + 0.6 visual; P0 caps at 59.
7. **Write the report** (template below) with concrete fixes; optionally produce a mock fix (value-adjusted crop, capsule redraft) and re-run the script to show deltas.

## 2. Report template

```markdown
# Game art review: <file> | <type> | <genre>

**One-line verdict:** <usable or not, biggest problem, what to fix first>

## Score
| Part | Score |
|---|---|
| Metric (script 40%) | xx/100 |
| Visual (agent 60%) | xx/100 |
| **Total** (≤59 if any P0) | xx/100 |

| Dimension | Weight | Score | Evidence |
|---|---|---|---|
| G1 Readability/silhouette | | | in grayscale, enemy vs ground value gap… |
| G2 Value & color hierarchy | | | |
| G3 Gameplay-first clarity | | | |
| G4 Art-direction consistency | | | |
| G5 UI/HUD | | | HUD edge coverage xx% |
| G6 VFX restraint | | | |
| G7 Lighting & mood | | | |
| G8 Character/environment design | | | |
| G9 Brand/key-visual appeal | | | |
| G10 Technical finish | | | |
| G11 AI artifacts/originality | | | |

## Issues
### P0 (blocks gameplay or store release)
1. **Issue** — evidence (location/metric) — fix (concrete values/method)
### P1 (clearly lowers quality)
### P2 (polish)

## Improvement plan (by priority)
1. … (e.g. compress background value to 30–45%, add a 2px warm outline to enemies)
## Tests
- Grayscale test: pass/fail · silhouette test · thumbnail test (184×69 / 120×45) · 2-second test
Attachments: grayscale.png, thumbnail.png, palette.png, thirds-overlay.png
```

## 3. Improvement recipes by genre

| Genre | Typical fix recipe |
|---|---|
| Shooter / BR | Desaturate & compress env value range (e.g. 25–60%), add enemy rim light/outline, keep crosshair zone ≤ 1 bright element, move HUD into 4 corners |
| MOBA | Ground textures −20% contrast, team-color rings under units, VFX intensity tiers (basic 30% / CC 60% / ult 100% brightness), skins keep base silhouette |
| ARPG / roguelike | Enemy projectiles one warm saturated hue on top render layer; player FX cooler & translucent; floor value band narrow; loot beams per rarity |
| Platformer / metroidvania | Platforms: +1 value step & outline; background −40% contrast, haze; hazards red/spiky shape family |
| Open world | Color script per biome/time; landmark silhouettes on skyline; haze for depth; HUD auto-hide |
| JRPG / gacha | Character color keys (one hero hue each); faces brightest value; UI badge cap (≤ 3 red dots); line weight spec |
| SLG / MMO / idle / card | Type scale ≤ 4 sizes, tabular numerals, icon shape coding, panel hierarchy (primary/secondary/tertiary backgrounds), empty-space gutters 8/16 px |
| Casual / match-3 | Pieces: hue + shape + value; board backing plate; background blurred; buttons ≥ 44 pt with 3D/press state |
| Horror | Lift blacks to ~8–15% luma; light pools on path/key objects; single accent hue; fog cards for depth |
| Racing / sports | Track/pitch neutral; kits contrast in value; motion blur periphery only; broadcast-style HUD |
| Pixel art | Choose one scale (e.g. 16/32 px tiles), 16–32-color palette with hue-shifted ramps, nearest-neighbour integer scale, selective outline |
| Wuxia/xianxia (武侠/仙侠) | Palette from azurite/malachite/cinnabar/ochre/moon-white (石青/石绿/朱砂/赭石/月白); mist layers; reduce particle glow by half; calligraphy only for logo |
| Ink wash (水墨) | 5 ink values + 1 red seal; ≥ 30% negative space (留白); dry brush at silhouette edges; no uniform digital texture |
| Capsule / icon | Logo ≥ 60% of small capsule width, heavy weight, outline/shadow for contrast; one character or symbol; test 120×45 on `#1b2838`; separate art for small capsule if needed |

## 4. Prompt templates (game art without AI slop)

Rules: state **genre, camera, gameplay function, value plan, palette count, shape language**, and what to avoid. Don't ask for text in-image (add logos later). Generate silhouettes/thumbnails first, detail second.

**Key art**
```
Key art for a <genre> game, <one-sentence hook>. Single hero <character> in a <pose verb> pose, strong readable silhouette against a <simple/darker/lighter> background, 3-value structure (dark frame, mid environment, brightest on face and <prop>). Palette: <dominant hue> dominant, <accent hue> accent only on <element>, max 4 hues. <rendering style: painterly / cel-shaded / ink wash>. Clear empty area at <top/bottom third> for the logo. No text, no watermark, no extra characters, no lens flares, no glowing particles everywhere, anatomically correct hands holding the <weapon>.
```
**In-game screenshot mock / environment**
```
<genre> gameplay scene, <camera: isometric / third-person over-shoulder / side-view 2D>, player character clearly separated from background by value and rim light, enemies use <warm red> accents, walkable path lit, background desaturated with atmospheric haze, HUD-free (UI added later), consistent <style> materials, low-frequency detail in background, no random props, no text.
```
**Character concept**
```
Character concept sheet, <role> for a <genre> game, shape language: <triangles=aggressive / circles=friendly / squares=sturdy>, readable as a black silhouette, detail clustered at <chest/head>, rest areas on <legs/arms>, front and 3/4 view consistent, neutral gray background, palette <3 colors + 1 accent>, no text, no extra limbs, symmetric gear consistent left/right.
```
**Pixel art**
```
<N>x<N> pixel art sprite, <subject>, exact 1-pixel grid, <16>-color limited palette with hue-shifted ramps, selective dark outline, no anti-aliasing, no gradients, no blur, transparent background, side view, idle pose.
```
(Then verify with `--type sprite --genre pixel`: unique colors ≤ palette, run mode = scale; redraw if fake.)

**水墨 / 仙侠**
```
Chinese ink wash (水墨) game scene, <subject>, five ink tones from deep black to pale wash, 40% empty rice-paper space, single cinnabar red accent on <seal/sun/cloak>, dry-brush edges on rocks, mist layers for depth, no neon glow, no gold particles, no text or fake calligraphy.
```
**Capsule (art only, logo added in a design tool)**
```
Wide 462x174 composition, one bold subject on the <left/right> third, large flat area on the other side for a heavy title logo, high value contrast against dark navy, readable when shrunk to 120x45, no small details, no text.
```
