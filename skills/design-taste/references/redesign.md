# Redesign / Refine Existing UI

Principle: **upgrade the look without breaking behavior.** Same routes, data, props, and tests unless the user asks otherwise.

## Workflow
1. **Inventory** — stack, CSS approach, component library, existing tokens, brand assets. Screenshot current state if possible.
2. **Audit** — run `audit.md` checks; list P0/P1/P2 with numbers.
3. **Direction** — write the direction block (`direction.md`). Preserve brand equities (logo, key color) unless told to rebrand.
4. **Tokens first** — consolidate literals into the 3-tier system (`tokens.md`). This alone fixes most inconsistency.
5. **Global layer** — typography scale, base colors, spacing rhythm, radius/shadow families.
6. **Components** — buttons, inputs, cards, nav, tables: states, sizes, focus.
7. **Layouts** — rework generic sections (hero + 3 cards, centered everything) into compositions that serve content.
8. **Polish** — motion, copy, empty/error/loading states, imagery.
9. **Verify** — re-run measurable checks; compare before/after numbers; screenshots at 3 widths; run existing tests/build.

## Rules
- Small, reviewable steps; keep diffs per layer.
- Don't swap frameworks or add heavy dependencies for looks.
- Don't delete functionality or content to make it "cleaner" without asking.
- Variants: "bolder" = more contrast/scale/color commitment; "quieter" = fewer colors, less decoration, more space; "distill" = remove elements until each remaining one earns its place.
- Report: what changed, before/after metrics, anything deferred.
