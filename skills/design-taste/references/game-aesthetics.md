# Game Aesthetics — rubric, genre profiles, asset rubrics

Game art is judged first by **whether the player can play**, then by whether it is beautiful and memorable. A gorgeous frame that hides the enemy is a P0. Use this file for any game image: screenshot, key art, store capsule, icon, UI screen, concept, VFX, sprite sheet, banner. Pair with `game-workflows.md` and `scripts/review_image.py --type <game type> --genre <g>`.

## Contents
1. Universal rubric (11 dimensions, 1–10)
2. Scoring and weights per asset type
3. Genre profiles (21)
4. Asset-type rubrics (9)
5. AI-artifact checks for game art
6. Metric ranges (from review_image.py)

---

## 1. Universal rubric (score each 1–10)

Anchors: **9–10** shippable AAA/top-indie quality, a deliberate choice visible everywhere · **7–8** solid, minor inconsistencies · **5–6** works but generic or cluttered · **3–4** readability or cohesion problems that affect play/marketing · **1–2** broken.

| # | Dimension | What 9–10 looks like | What 3–4 looks like | Quick test |
|---|---|---|---|---|
| G1 | **Readability & silhouette** | Every gameplay-relevant shape (player, enemy, pickup, hazard) recognisable as a black fill; distinct per role/class | Characters blend with props; silhouettes merge in groups | Fill subjects black on white; Otsu mask in `grayscale.png` |
| G2 | **Value & color hierarchy** | 3–4 clear value groups; brightest/most saturated = most important; background compressed into a narrow value band | Even mid-grey everywhere; highlights scattered; background as busy as foreground | Grayscale + 3-value posterize; squint |
| G3 | **Gameplay-first clarity** | Player always found in < 0.5 s; enemy vs ally vs neutral coded by color + shape; interactive objects share one affordance language (outline, glow, color) | Interactables look like decoration; enemy shots same color as background VFX | "Where am I / what hurts me / what can I use" in 2 s |
| G4 | **Art-direction cohesion** | One rendering rule set (shape language, edge treatment, material, brush, palette) across characters, env, UI, VFX | Asset-store mix; realistic props next to toon characters; UI from another game | Put 4 random assets side by side |
| G5 | **UI/HUD clarity & density** | HUD grouped at edges, consistent grid, priority by size/contrast, center clear for play; density fits genre | Panels everywhere, 5 type styles, critical info same weight as flavor | HUD coverage %; read priority order aloud |
| G6 | **VFX discipline** | Effect intensity ∝ gameplay importance; clear anticipation → impact → dissipation; hitboxes match visuals | Particle soup; ultimate looks like basic attack; bloom hides enemies | Freeze frame mid-fight; count competing bright spots |
| G7 | **Lighting & atmosphere** | Light guides the path and focal point; atmospheric perspective separates depth layers; mood matches moment (color script) | Flat uniform light; or so dark that gameplay is guessed | Blur the image: does light still point to the goal? |
| G8 | **Character & environment design** | Strong shape language matches personality/role; detail clustered with rest areas; environment as stage not wallpaper | Detail evenly sprinkled ("detail everywhere = no detail"); generic armor/fantasy kit | Detail-density map (thirds-overlay heat) |
| G9 | **Brand / key-art appeal** | One iconic hook (pose, prop, color) memorable at thumbnail; logo integrated; genre readable instantly | Collage of everything; logo illegible; could be any game | 120×45 / 128 px thumbnail test |
| G10 | **Technical polish** | Clean edges, consistent texel/pixel density, no stretched textures, no aliasing shimmer, crisp UI text | Mixed resolution, blurry upscales, compression mush, mixed pixel scales | 200% zoom on edges; unique-color count for pixel art |
| G11 | **AI-artifact & originality** | No generation tells; consistent design across views; IP-safe | See §5 | §5 checklist |

## 2. Weights per asset type (visual score = Σ score × weight / Σ weight × 10)

| Dim | screenshot | key-art | capsule/icon | game-ui | concept | VFX | sprite | banner |
|---|---|---|---|---|---|---|---|---|
| G1 Readability | 3 | 2 | 3 | 1 | 2 | 2 | 3 | 2 |
| G2 Value/color | 2 | 2 | 3 | 2 | 2 | 2 | 2 | 2 |
| G3 Gameplay clarity | 3 | 0.5 | 0 | 2 | 1 | 3 | 2 | 0.5 |
| G4 Cohesion | 2 | 1.5 | 1 | 2 | 2 | 1.5 | 2 | 1 |
| G5 UI/HUD | 2 | 0 | 0 | 3 | 0 | 0 | 0 | 0.5 |
| G6 VFX | 1.5 | 0.5 | 0 | 0.5 | 0.5 | 3 | 0.5 | 0 |
| G7 Lighting | 1.5 | 2 | 1 | 0.5 | 2 | 1.5 | 0.5 | 1 |
| G8 Char/env design | 1.5 | 2 | 1 | 0 | 3 | 0 | 2 | 1 |
| G9 Brand appeal | 0.5 | 3 | 3 | 1 | 1 | 0 | 0.5 | 3 |
| G10 Tech polish | 1.5 | 1.5 | 2 | 2 | 0.5 | 1 | 2 | 1.5 |
| G11 AI/originality | 1 | 1.5 | 1.5 | 0.5 | 1.5 | 0.5 | 1 | 1.5 |

Total = 0.4 × metric score (script) + 0.6 × visual score. Any P0 caps the total at 59.

**P0** (blocks play or sale): player/enemy unreadable; critical HUD unreadable or hidden; capsule title illegible at 120×45; store-rule violations (Steam base capsules: only artwork + game name + official subtitle — no review scores, awards, discount or CTA text); wrong aspect/size; broken AI anatomy on the hero. **P1**: muddy value structure, palette without a dominant, HUD clutter, inconsistent style, VFX noise, mixed pixel scale. **P2**: polish — edge cleanup, color temperature drift, minor spacing.

## 3. Genre profiles

Format: **weights shift** (dims to up-weight ×1.5) · **palette/value** · **HUD convention** · **common mistakes** → **recommendations**. Numeric targets match `--genre` keys in `review_image.py` (rms value contrast range / max significant colours / max edge-HUD coverage).

### RPG
- **Western RPG** (`rpg-western`; 40–75 / 7 / 25%) · ↑G4 G7 G8 · grounded desaturated materials, one magical/danger accent, warm torch vs cool ambient. HUD: portraits + party at a corner, hotbar bottom-center, minimap top-right; menus diegetic-flavored (parchment, leather) but legible. Mistakes: brown-grey sameness; loot icons unreadable; dialogue boxes over faces → Value-group the scene, rarity color on frame not whole icon, 24px+ body text.
- **JRPG** (`jrpg`; 45–85 / 8 / 30%) · ↑G1 G9 · higher saturation, cel/anime shading, bold character colors against softer backgrounds. HUD: turn order bar, command menu; stylised menus (Persona-like diagonals) acceptable if hierarchy holds. Mistakes: all-pastel = no focus; menu style overriding readability → keep one hero hue per character, test menus in grayscale.
- **ARPG / looter** (`arpg`; 45–85 / 7 / 25%) · ↑G1 G3 G6 · dark grounds, bright skills, monsters ≥ 2 value steps from floor; loot rarity color code (grey/white/blue/yellow/orange…) strictly consistent. HUD: health/resource orbs or bars at bottom corners, skill bar center. Mistakes: AoE spam hides enemy telegraphs → cap VFX alpha, enemy telegraphs on top layer with unique color (red/orange).

### MOBA (`moba`; 45–85 / 8 / 25%)
↑G1 G3 G6. Map low-to-mid saturation, champion/skill effects high contrast; team colors (ally blue / enemy red) never overridden by skins; VFX importance tier = gameplay impact (ultimate > CC > basic), hitbox edges visible (Riot's clarity guidelines). HUD: minimap corner, ability bar bottom-center, scoreboard top. Mistakes: skins that change silhouette or ability shapes; bloom over terrain → skin "clarity parity", ground decals with crisp edges.

### FPS/TPS shooter (`shooter`; 35–80 / 6 / 15%)
↑G1 G3 G5. Enemies must out-contrast the environment (rim light, value separation), environment detail low-frequency at combat distance; crosshair area clean. HUD: health/armor bottom-left, ammo bottom-right, minimal; diegetic acceptable for immersive single-player, competitive needs clear non-diegetic. Mistakes: camo realism making enemies invisible; full-screen post FX → reserve saturated hues for threats/objectives.

### Battle royale (`battle-royale`; 40–80 / 7 / 20%)
↑G1 G3. Readable at 200 m: characters vs foliage via value not just hue; circle/storm color unique; loot rarity visible at distance. HUD: minimap + zone timer top-right, squad bottom-left, inventory compact. Mistakes: cosmetics that camouflage → limit dark/foliage-matching skins or add rim light.

### Open world (`open-world`; 35–75 / 7 / 15%)
↑G7 G8 G9. Atmospheric perspective (near: darker/saturated; far: lighter/cooler/desaturated), landmark silhouettes ("weenies") visible from far, color script by region/time of day. HUD minimal and hideable; compass bar top or minimap. Mistakes: icon-carpet maps; uniform foliage noise → cluster detail, carve visual paths with value.

### 2D platformer / metroidvania (`platformer`; 45–90 / 7 / 15%)
↑G1 G3. Walkable surfaces clearly separated from background (value, saturation, outline); 3 layers (fore/mid/back) with background contrast compressed; hazards share one shape/color language (spikes, red). HUD tiny, top-left. Mistakes: decorative foreground occluding the player; background as contrasty as platforms → parallax layers −30–50% contrast, desaturate back.

### Pixel art (`pixel`; 45–95 / 16 / 20%)
↑G4 G10. Limited palette (often 16–32 colors, ramps with hue shift), single consistent pixel scale, integer scaling with nearest-neighbour. Script: `unique_colors_exact` (clean ≤ 64–256), `pixel_run_mode` = upscale factor. Mistakes: mixed pixel sizes, rotated/sub-pixel sprites, bilinear blur, "pillow shading", AI "pixel-ish" noise → redraw at native res, enforce palette, check run lengths.

### Anime / 二次元 gacha (`gacha`; 45–85 / 8 / 35%)
↑G8 G9 G4. Character illustration is the product: face/eyes focal, clean lineart, consistent rendering across banners; rarity color (gold/purple/blue) consistent; UI ornate but layered (currency top-right, events left rail, main CTA bottom-right). Mistakes: UI overload (red dots everywhere), inconsistent line weight between artists, AI hand/hair errors → define a line/shading spec sheet, cap simultaneous badges.

### SLG / 4X strategy (`slg`; 35–70 / 8 / 40%)
↑G5 G3. Dense info is OK if hierarchy is strict: map low-sat, faction/units high-sat, resources distinguished by **icon shape + color**; legible at every zoom level (LOD icons). Mistakes: equal-weight panels, tiny numbers, color-only faction coding → tabular numerals, faction color + emblem, zoom-dependent detail.

### Card / CCG (`card`; 45–85 / 8 / 40%)
↑G1 G4 G5. Frame system consistent; cost/attack/health numerals top priority (high contrast, bold); illustration readable at hand-size (~120 px tall); rarity/faction coded by frame color + gem shape. Mistakes: illustration busy behind text; inconsistent illustrators → card text on solid plate, art brief with focal-area mask.

### Casual / puzzle / match-3 (`casual`; 40–85 / 8 / 30%)
↑G1 G3 G9. High saturation is fine but each piece double-coded (hue + shape) for color-blind play; rounded shape language; big tappable buttons; board strongly separated from background. Mistakes: candy-colored everything equal; background competing with board → blur/darken backgrounds, board frame.

### Idle / incremental (`idle`; 40–80 / 7 / 45%)
↑G5. Number hierarchy (main currency biggest), clear affordable/unaffordable states, long-session comfort (no constant flashing). Mistakes: red-dot spam, unformatted big numbers → suffix notation (K/M/B or 万/亿), muted idle animation.

### Horror (`horror`; 25–70 / 5 / 10%)
↑G7 G3. Low-key lighting, large dark masses, but key objects and path still readable (light pools, rim); one warning hue (blood red, sickly green); fog for depth. HUD minimal/diegetic. Mistakes: pure black crushed (clip_dark > 30%) hiding everything; jump-scare VFX noise → keep shadows at value 10–20 not 0, light the path.

### Racing (`racing`; 40–85 / 7 / 20%)
↑G3 G6. Track edges contrast with road; motion blur only at periphery; car readable vs. sky/track; speed via streaks, FOV. HUD: speed/rpm bottom-right, position/lap top-left, minimap. Mistakes: bloom/lens flare in braking zones → clamp post at apexes.

### Sports (`sports`; 40–80 / 7 / 20%)
↑G1 G5. Team kits mutually exclusive in hue **and value**; neutral pitch; ball/puck highest contrast; broadcast-style scorebug top-left. Mistakes: similar kits; scoreboard clutter → kit clash rules, broadcast templates.

### Roguelike / roguelite (`roguelike`; 45–90 / 7 / 25%)
↑G1 G3 G6. Bullet/telegraph layer always on top with unique hue; room floors compressed in value; boons/relics distinguished by icon shape; run-to-run color identity per biome. Mistakes: bullet-hell soup; similar relic icons → enemy projectiles high-sat warm, player FX cool/desaturated.

### MMO (`mmo`; 40–75 / 8 / 40%)
↑G4 G5. Dense HUD but default layout grouped; nameplates readable in crowds; armor sets styled within one language (avoid clown-suit mixes; transmog helps). Mistakes: 20 hotbars default; inconsistent outsourced assets → UI layout presets, art bible with material/shape rules.

### 武侠 / 仙侠 Chinese styles (`wuxia`; 35–80 / 6 / 30%)
↑G4 G7 G9. Traditional pigment palette (石青 azurite, 石绿 malachite, 朱砂 cinnabar, 赭石 ochre, 月白, 藤黄), mist layering, flowing cloth lines for motion; architecture with correct dynasty details. Calligraphic fonts only for titles/logos, readable sans/serif (思源宋体/黑体) for UI body. Mistakes: generic "Chinese red + gold" everywhere; glowing particle overload in 仙侠; wrong-era props; dense calligraphy UI → restrained palette, 留白, one hero color.

### 水墨 ink style (`ink`; 40–95 / 4 / 20%)
↑G2 G4. Value is everything: "墨分五色" (焦/浓/重/淡/清 — five ink tones) as the value structure; ≥ 30% 留白; one 朱红 accent (seal, sun, blood); dry-brush texture at edges, wet washes for depth. Mistakes: digital uniform brush texture, gray mush with no 焦墨 accents, too many colors → enforce 3–5 ink values + 1 red, let paper show.

## 4. Asset-type rubrics (add to universal dims)

| Asset | Must pass | Typical fails |
|---|---|---|
| **In-game screenshot** (store/App Store) | Shows real gameplay (Apple 2.3.3: app in use, not just title art); player + goal readable; HUD not hiding action; first 1–3 images sell the core loop | Menus/loading screens as screenshots; UI debug text; 4K detail invisible at store size |
| **Key art / poster** | One focal hero, strong silhouette + value design, space for logo, genre instantly legible, survives 320×180 | Character collage; logo pasted on busy area; no value hierarchy |
| **Store capsule / icon** | Steam small capsule 462×174 auto-scaled to 184×69 and 120×45 — logo must stay readable, may use different art than header; base capsules contain only art + game name + official subtitle; icon: one shape, 2–3 values, no tiny text | Thin title type, multiple characters, low contrast vs Steam navy `#1b2838`, review quotes/awards |
| **UI screen** | Grid, type scale ≤ 3–4 sizes, clear primary action, state coverage, consistent icon style, localisation room (CJK +30% width) | Ornament over function, inconsistent frames, color-only states |
| **Character concept** | Silhouette tells role/personality; shape language (circle=friendly, square=sturdy, triangle=dangerous); detail clusters + rest areas; turnaround consistency | Detail everywhere, generic armor, unreadable at gameplay scale |
| **Environment concept** | Clear composition path, foreground/mid/back values, light as navigation, landmark, gameplay space indicated | Matte-painting beauty with no playable space; uniform noise |
| **VFX** | Anticipation→impact→dissipation, shape matches hitbox, intensity ∝ importance, color by team/element, readable over all biomes | Bloom soup, alpha overdraw hiding enemies, too long linger |
| **Sprite / pixel sheet** | Consistent pixel scale + palette, frame registration (no jitter), readable at 1×, clean outlines/selective outline | Mixed resolution, anti-aliased edges against transparency, orphan pixels |
| **Marketing banner** | Platform-safe zones, logo + one hook + (optional) one CTA where allowed, readable on mobile, matches key-art system | Text walls, cropped faces, illegal claims/ratings on Steam base assets |

## 5. AI-artifact checks for game art
- Hands/fingers, weapon grips, symmetric gear that doesn't match left/right, belts/straps that dissolve.
- Gibberish text on UI, signs, logos; fake glyphs in "Chinese" calligraphy.
- Inconsistent design across views/frames (costume changes, extra limbs in sprite frames).
- "Everything glows": rim light on every edge, particle dust everywhere, saturation p95 ≥ 0.95.
- Detail without function: filigree everywhere, no rest areas, mushy micro-noise (shape_edge_retention low with high edge density).
- Pixel-art fakes: non-integer pixel grid, thousands of unique colors, blurry edges.
- Perspective/scale errors in environments; floating props; tiles that don't tile.
- Style drift that resembles a specific living artist or existing IP → originality/legal risk.

## 6. Metric ranges (review_image.py game types)

| Metric | Healthy | Concern |
|---|---|---|
| `value_bands` max band | ≤ 0.70 (low-key horror/ink may reach 0.85) | > 0.85 → P1 monotone value |
| `value_levels_used_5` | 3–5 | ≤ 2 |
| `figure_ground_gap` (Otsu class mean gap) | ≥ 80 | < 60 → P1 |
| `shape_edge_retention` (edges surviving 4× blur) | ≥ 0.40 | < 0.30 → noisy micro-detail, weak big shapes |
| `hud_edge_coverage_est` | ≤ genre max (10–45%) | above → clutter |
| `hud_center_clutter_est` | < 0.08 | > 0.15 → UI over play area |
| `color_bins_95pct` (5-bit) | ≤ 600 stylised, ≤ 3000 realistic | very high on stylised art = noise |
| `unique_colors_exact` (pixel/sprite) | ≤ 64–256 | > 256 → interpolated/AI pixel art |
| thumbnail `contrast_rms` | ≥ 40 | < 35 on capsule/icon/key art → P0 |
| thumbnail `contrast_retention` | ≥ 0.85 | < 0.75 → P0 for capsule/icon |

All metrics are proxies; HUD and silhouette estimates can be fooled by architecture (straight lines) or busy textures. Always look at `grayscale.png` and `thumbnail.png` yourself.
