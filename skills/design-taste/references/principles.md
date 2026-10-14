# Visual Principles

## Color
- 60/30/10: dominant neutral surface, secondary surface/text, ~10% accent.
- Accent marks *interactive or important* things only. If everything is accented, nothing is.
- Never pure `#000` text on pure `#fff` for long reading; use off-black/off-white.
- Gray text on colored backgrounds: use a tint/shade of the background hue instead.
- Gradients: subtle and purposeful (lighting, depth), not as the brand.
- ❌ purple/indigo→blue hero gradient, neon glow, rainbow charts. ✅ one confident color field, tinted neutrals.

## Typography
- Modular scale (e.g. 1.2–1.333 ratio): 12 / 14 / 16 / 20 / 24 / 32 / 48 / 64. Use ≤ 6–8 sizes.
- Weights: 2–3 (e.g. 400 / 500 / 700). Hierarchy = size + weight + color, not all-caps everywhere.
- Max line length ~ 65ch; avoid justified text on web; avoid widows in headings (`text-wrap: balance`).
- ❌ everything 14px medium gray. ✅ clear step from H1 → body → caption.

## Spacing & layout
- 4/8px base scale: 4, 8, 12, 16, 24, 32, 48, 64, 96, 128.
- Proximity: inner padding < gap between groups < section spacing.
- Use a grid (12-col or content-based); align edges. Asymmetry is fine when intentional.
- Break the "hero + 3 cards" template: try split layouts, big type, lists, tables, bento with varied spans, overlapping elements.
- Don't wrap everything in cards. Use whitespace, dividers, or background shifts to group.
- Responsive: design mobile and desktop deliberately; never just shrink.

## Radius
- One family: e.g. 4 (inputs, tags) / 8 (cards) / 9999 (pills). Nested radius = outer − padding.
- Sharp radii read technical/editorial; large radii read soft/consumer. Pick per tone; don't mix arbitrarily.

## Shadow & depth
- ≤ 3 elevation levels (raised, overlay, modal). Shadows soft, low-opacity, layered; tinted toward background hue.
- Prefer borders or surface tone shifts for flat/minimal looks. No shadow + border + gradient on the same card.
- Dark mode: elevation by lighter surface, not shadow.

## Motion
- Purpose: feedback, continuity, orientation. Not decoration.
- Durations: 100–150ms (micro: hover/press), 200–300ms (enter/exit), ≤ 500ms (page-level).
- Ease-out for entering, ease-in for exiting; avoid linear for UI; avoid elastic/bounce unless playful tone.
- Animate `transform` and `opacity` only; never width/height/top/left for UI transitions.
- One well-orchestrated moment (e.g. staggered page load) beats scattered effects.
- Always honor `prefers-reduced-motion`.

## Iconography & imagery
- One icon set, one stroke weight, sized to the text it sits beside. No emoji as UI icons.
- Real or purpose-made imagery; consistent crop, ratio, and treatment.

## Copy
- Specific verbs and nouns. Ban: "Elevate", "Seamless", "Unleash", "Next-gen", "Revolutionize".
- Buttons say what happens ("Create invoice", not "Submit").
- Error messages: what happened + how to fix.
