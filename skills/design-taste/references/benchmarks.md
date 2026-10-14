# Benchmarks — what the best corporate homepages actually do

Data source: two 2026-10 crawls of homepages (headless Chromium, 1440×900). One covered the global top-100 companies by market cap, the other the top-100 tech/internet/software companies by market cap. After deduplication, 147 sites were scored with the same formula used by `scripts/review_site.py`: 45% metric score plus 55% visual score. Per-site rows are in `scripts/benchmarks.csv`. The scores are heuristic. Use them to compare sites, not as absolute grades.

## 1. Top 10 breakdowns

### Global top-100 run
| # | Site | Palette | Type | Layout / hero | CTA |
|---|---|---|---|---|---|
| 1 | SpaceX 89.1 | Black canvas, white text, 9 colour values total | Condensed DIN caps, 60/16px | Full-bleed photo/video per section, one line of copy | Text-link "Watch"/"Learn more" |
| 2 | Eli Lilly 87.7 | B/W photography, blush + white + brand-red section bands | Extra-wide sans 100px + condensed serif italic for emphasis, body 20px | Huge headline over monochrome photo; sections switch background | One red button per band |
| 3 | Novartis 86.6 | White, near-black, a single orange gradient blob as signature | Own sans, 62/16px, 3 text colours | Airy left-aligned hero, lots of whitespace | Outline button |
| 4 | Inditex 86.4 | Pure B/W/grey, 6 colours | Thin sans, 56/18px | Fashion photo grid, 0 radius, 0 shadow | Underlined text links |
| 5 | J&J 85.6 | White + red used only on CTA band and links | Proprietary serif display + sans text, one weight | Left text / right image, strict 60-30-10 | Red pill CTA |
| 6 | JPMorgan Chase 83.8 | Warm white, navy text | Editorial serif 72px, "01" numbering | Annual-report-magazine layout with portraits | Text link + arrow |
| 7 | Meta 83.5 | Lifestyle photo, neutral UI | 48/18px, 0 shadows | One big image + one sentence | Single pill |
| 8 | Roche 83.1 | Warm off-white, blue only on buttons | Roche Sans/Serif, body 20px | Generous margins, calm cards | Blue button |
| 9 | IHC 82.9 | Aerial photo, white type | Thin 92px display | Full-bleed hero; stock ticker shrunk to a footnote | Minimal |
| 10 | Alphabet 82.4 | 4 text colours, 0 shadows | 2 font sizes only | IR text page: perfect tokens but weak hierarchy (visual 7) | Links |

Apple came 19th: its visual score was 9, but 33 colour values and 18 gradients from product tiles pulled the metric score down. A high colour count that comes from deliberate product zoning is not the same thing as chaos.

### Tech top-100 run (unique entries)
| # | Site | What it does |
|---|---|---|
| 2 | Robinhood 85.7 | Best metric score (98.9): 12 colours, 3 font sizes, 0 gradients. Greyscale portraits, a 72px serif headline (Martina Plantijn) and one neon-yellow accent used only on the CTA. Long dark page made of real product screens. |
| 5 | IBM 81.7 | Carbon system. IBM Plex Sans, white canvas, blue text links, left-aligned grid. Order instead of decoration. |
| 6 | Netflix 80.8 | Dark poster wall, one red CTA, one goal (sign up). |
| 7 | Intel 79.6 | Dark canvas, a 240px display headline, restrained blue. |
| 9 | AppLovin 78.8 | Photo collage with a centred 80px headline, body contrast 19.9. |
| 10 | Seagate 78.6 | Deep green tone with product photography, one green CTA, 14 colours. |

**Shared pattern:** one focal point per viewport; a headline ≥56px with body text 16–20px; neutral colours plus one accent; photography or real UI rather than 3D renders; section changes made with background colour, not with shadows.

## 2. Targets (median of the top-20 vs bottom-20 sites, both runs combined)

| Metric | Top 20 | Bottom 20 | Target |
|---|---|---|---|
| Colour values (text+bg+border) | 14 | 29 | ≤ 15 |
| Font families | 2 | 2 | ≤ 2 |
| Font sizes used ≥2× | 6.5 | 9 | ≤ 7 |
| Font weights | 2.5 | 3.5 | ≤ 3 |
| Radius values | 3 | 6 | ≤ 3 |
| Shadow values | 0.5 | 3 | ≤ 1 |
| Gradients | 0 | 1 (tech bottom: 8) | ≤ 1 |
| Hero headline | 61px | 48px | ≥ 56px |
| Body size | 16px | 14px | ≥ 16px (marketing) |
| Hero/body ratio | 3.6 | 3.2 | ≥ 3.5 |
| Body contrast (weighted) | 16.2 | 10.2 | ≥ 12 |
| Text below AA | 0% | ~2–10% | 0% |

Font-family count does not separate good sites from bad ones. **Colour count, gradients and shadows separate them most:** the best sites win by using less.

## 3. Sub-industry playbooks

Each recipe is a starting point. Map it onto `tokens.md` semantic names.

**Pharma / healthcare (best category, avg 76).** Humanist photography, often monochrome; a serif or extra-wide display face; warm whites.
`--canvas:#FBF8F5; --text:#1A1A1A; --accent:<brand red/orange/blue>; display 80–100px serif or wide sans, body 18–20px; radius 0–4px; shadows none; sections alternate canvas / tint-of-accent at 6%.`

**Luxury / fashion retail (LVMH, Inditex).** Photography carries the page; the type stays quiet.
`palette = black/white/1 grey; thin or high-contrast sans, uppercase nav at 11–12px with 0.1em tracking; radius 0; shadows 0; CTA = underlined text.` Avoid badges and price stickers. Mass retail (Walmart, Costco, Amazon) is promo-driven by business necessity, so do not benchmark taste against it.

**Finance (avg 66.5, polarised).** The winners (JPMC, Citi, HSBC) look like magazines. The losers (BofA, Chinese state banks, TD) look like portals.
`navy or brand ink on warm white; editorial serif headline 56–72px + sans body 16–18px; numbered sections ("01"); one login entry in the nav, not a login card in the hero; tabular-nums for rates.`

**Internet / consumer platforms (avg ~70).** Make the product itself the homepage (Airbnb, Spotify, Robinhood), or use one image with one sentence (Meta, Booking).
`one accent (signature colour) used only on the primary CTA; real UI screenshots; body 16–18px; pill CTA; dark long-page sections allowed if contrast ≥12.`

**Software / cloud (worst category, avg 61.6).** The typical failures are announcement bars, Gartner badges, multicolour CTAs and gradient meshes. IBM is the model to copy.
`one sans family (Plex-like) in 3 weights; 6 sizes (14/16/20/28/40/56); accent = one blue; radius 0–4px; shadow only on popovers; at most 1 announcement bar; no chat bubble above the fold.`

**Semiconductors (lowest visual score, avg 65).** Most sites use dark canvases with blue/purple glow and chip renders, so brands are hard to tell apart. Intel, KLA and AMAT show the alternatives: one giant headline, one big number, or real fab photography.
`if dark: --canvas:#0B0D10, text #F2F2F2, accent = brand colour (not cyan glow), 0 gradients; prefer photo of real hardware/fab; headline ≥72px.`

**Hardware / devices (avg 66).** Photography-led product staging works (SpaceX, Apple, Seagate, Nokia). Catalogue grids fail (TE, Keyence).
`neutral canvas per product tile; one product + one line per viewport; secondary nav for categories, never a mega-grid on the homepage.`

**Aerospace / defence (avg ~80).** Full-bleed aircraft or launch imagery with almost no UI. White condensed caps, 3 text colours, zero shadows.

**China vs US (tech run).** Chinese sites score better on metrics (78 vs 70: fewer colours, almost no gradients) but worse on visuals (55 vs 62). The causes are IR/portal templates, low body contrast (median 8.5) and 12–14px Chinese body text. Fixes: CJK body text at 16px and line-height 1.7–1.8; use `"PingFang SC","Noto Sans SC"` after the Latin face; give the hero one message instead of a news carousel; text contrast ≥7:1. Xiaomi (white product staging with one line of copy) is the closest Chinese example of the Apple pattern. US sites have the highest scores (SpaceX, Robinhood) and also the lowest (Palo Alto, Snowflake, Fortinet).

## 4. Common mistakes (with examples)

- **Portalisation:** every channel above the fold, with link walls and icon walls (ICBC, Bank of China, PetroChina, BofA, Keyence). Fix: one hero, with channels moved into the nav.
- **Promo stacking:** price badges, countdowns and multicolour labels (Dell, Costco). In B2B the equivalent is announcement bar + award badge + event banner + chat bot (ServiceNow, Snowflake, Fortinet).
- **The "AI-tech" template:** dark canvas, cyan/purple glow, particles, chip renders (AMD, Arm, Palo Alto, Synopsys, Cambricon, CrowdStrike). It matches the slop list in SKILL.md.
- **Token debt:** Palo Alto uses 20 font sizes, Snowflake 42 colours and 16 sizes, Shopify 11 radii and 10 shadows, TE Connectivity 9 font families, BofA and Santander 7 families.
- **Thin text on photos:** grey-toned imagery under thin white text (RTX, AMAT; on Alibaba 67% of body text fails AA; NAURA's body contrast is 1.7).
- **Overlays covering the hero:** cookie walls, region pickers, commemorative pop-ups (Delta TH, Murata, TotalEnergies).
- **Perfect tokens, no hierarchy:** Alphabet IR and Berkshire Hathaway get high metric scores and low visual scores. Restraint alone is not design.
- **Undersized body text:** 11–13px body text (Bank of China, Qualcomm, Apple's 12px nav-heavy text) costs accessibility points.
