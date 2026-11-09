# Craft floor

Load after the direction is settled (Step 2 / B4), and apply without announcing a checklist. A pinned brief or an existing design system overrides anything here; habit does not. These are **checks on the built result**, not intentions — run them together in each batched look at the render (one screenshot pass at ~375 and ~1280, then fix what that pass shows), not as a screenshot after every tweak.

## Verify (B6 and every polish pass)

- **Contrast:** body and placeholder text ≥ 4.5:1, large text ≥ 3:1. On colored surfaces, tint secondary text from that hue; never gray-on-color.
- **Depth:** shadows carry an offset and a soft blur. A zero-offset colored halo is decoration, not elevation.
- **Spacing:** tight groups, generous separation; more space above a heading than below it. Read computed values when you can.
- **Type:** body measure ~65–75ch; display max about 6rem; tracking floor about −0.04em; balanced headings; obvious scale and weight steps. Run the **real** copy at every breakpoint and fix overflow.
- **Motion:** one authored moment, not scattered effects and not the same entrance on every section. Exponential ease-out from an already-visible default. Prefer `transform` / `opacity` (and, when smooth, blur / mask / shadow); never animate layout properties.
- **States:** hover, focus, active, disabled, loading, empty, error — plus real content, working controls, responsive composition, keyboard focus.
- **Browser surfaces:** theme the parts you did not draw — text selection, caret, scrollbars, focus rings, underline offset, tabular numerals. Leaving them on browser defaults is the cheapest “assembled, not designed” tell.
- **Copy:** product language. Controls name the action; errors name the problem and the recovery.
- **Coverage:** every brief requirement is present and findable within seconds.

## Refuse (category defaults — the brief can earn them back; a free axis cannot)

Page scaffolds:

- Same-size cards of icon + heading + text as the page structure. Nested cards are always wrong.
- The hero-metric template: big number, small label, supporting stats, accent bar.
- A kicker / eyebrow above a heading. **Ban, not a default:** no brief earns it back — delete the label and let the heading speak.
- Section numbers (01 / 02 / 03) unless the sequence itself carries information the reader needs.
- A modal for a task that needs neither interruption nor protected focus.

Surface habits:

- Gradient text. Emphasis comes from weight or size.
- Glass and blur as decoration rather than a specific effect.
- A colored `border-left` / `border-right` thicker than 1px on cards, list items, callouts, or alerts.
- Hard offset shadows (`box-shadow: 4px 4px 0`) outside a world that is actually neobrutalist.
- Sparklines, progress rings, and soft-shadowed rounded rectangles standing in for content.
- Monospace as a costume for “technical” rather than for code, data, or measurement.
- A system display face (Impact, Arial Black, the platform sans) as the display voice of an own-world page — source a face that matches the lettering.
- Unicode glyphs or emoji standing in for an icon system. Icons are drawn (library or authored SVG), one stroke and weight.
- Geometric masks (circle / polygon / radial cutout) standing in for organic photographic contours — omit rather than fake.
- Light or dark picked by category. Pick from the use scene: who, where, under what ambient light.

The floor holds the mechanics; it never picks the direction. With every check green, spend the page on the committed world — when torn between refined and committed, **commit**.
