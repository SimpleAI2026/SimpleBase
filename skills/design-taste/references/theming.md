# Theming Slides, Docs, Reports

A theme = **palette (4–6 colors with roles) + font pairing (heading/body) + a few layout rules.** Apply it consistently across every page/slide.

## Workflow
1. Identify artifact and context (pitch deck, internal report, teaching doc, HTML handout).
2. Offer 3–6 named theme options (name + one-line mood + palette swatches + fonts), or derive from brand if given. Let the user pick when practical; otherwise choose and state why.
3. Apply: set theme variables once (master slide, doc styles, CSS vars), never per-element literals.
4. Check contrast of text on every background used; check charts use the palette in a fixed order.

## Theme recipe
| Role | Example |
|---|---|
| Background | warm off-white / deep navy |
| Surface | slightly offset from background |
| Text primary / secondary | off-black / muted |
| Accent 1 (primary emphasis) | one saturated hue |
| Accent 2 (data/secondary) | complementary or analogous hue |
| Fonts | heading face with character + highly legible body face |

Mood starters (create your own names/values): *Ocean depths* (navy, teal, sand), *Forest* (deep green, moss, cream), *Sunset* (coral, plum, peach), *Modern minimal* (graphite, white, one signal color), *Midnight tech* (near-black, electric cyan), *Desert rose* (dusty pink, terracotta, clay).

## Slides specifics
- One idea per slide; title states the takeaway.
- Big type (titles 36–54pt, body ≥ 18pt), max ~6 lines.
- Consistent grid and margins; image bleeds intentional.
- Charts: remove chart junk, label directly, highlight the one series that matters in the accent.

## Docs/reports specifics
- Clear heading hierarchy (≤ 3 levels), 60–75ch measure, consistent table styling, captioned figures.
