# Audit / Review

Goal: find what most hurts usability and perceived quality, ranked, with evidence. Review first; change code only if asked.

## 1. Measurable checks (run these, report numbers)
Adjust globs to the project.
```bash
# distinct hex colors (target: small, mostly via tokens)
rg -o --no-filename '#[0-9a-fA-F]{3,8}\b' src | tr 'A-F' 'a-f' | sort | uniq -c | sort -rn
# distinct font sizes (target ≤ 6–8)
rg -o --no-filename 'font-size:\s*[^;]+|text-(xs|sm|base|lg|[0-9]?xl)|text-\[[^]]+\]' src | sort | uniq -c | sort -rn
# radii, shadows, z-index, arbitrary Tailwind values
rg -o --no-filename 'border-radius:\s*[^;]+|rounded(-[a-z0-9]+)?' src | sort | uniq -c | sort -rn
rg -o --no-filename 'box-shadow:\s*[^;]+|shadow(-[a-z]+)?' src | sort | uniq -c | sort -rn
rg -o --no-filename '\w+-\[[^]]+\]' src | sort | uniq -c | sort -rn | head -40
# font families, !important, inline styles
rg -o --no-filename 'font-family:\s*[^;]+' src | sort | uniq -c
rg -c '!important' src ; rg -c 'style=\{\{|style="' src
```
Thresholds (guidance): > ~12 distinct literal hex values, > 8 font sizes, > 3 radii, > 3 shadows, > 2 font families, or many arbitrary values ⇒ token debt.

Also check: contrast of real text/background pairs (WCAG AA 4.5:1 / 3:1), tab through for focus visibility, viewport 375 / 768 / 1280 for overflow, `prefers-reduced-motion` handling, image `alt`, form labels.

## 2. Prioritized checklist

**P0 — broken / blocking (fix first)**
- Text contrast below AA; invisible focus; keyboard traps.
- Layout breaks or horizontal scroll on mobile; overlapping/clipped content.
- Primary action unclear or hidden; missing error/loading/empty states that leave users stuck.
- Touch targets < 44px for primary controls; forms without labels.

**P1 — quality & consistency**
- No clear direction / generic AI look (see slop list in SKILL.md).
- Token debt (counts above); inconsistent spacing rhythm; mixed radius/shadow families.
- Weak hierarchy: too many equal-weight elements, too many font sizes/colors.
- Cards-in-cards, excessive borders, decorative gradients/glass.
- Motion on layout properties, no reduced-motion, durations > 500ms.

**P2 — polish**
- Copy clichés, placeholder names, emoji icons.
- Optical alignment, icon sizes vs text, `text-wrap: balance` on headings, tabular numerals.
- Micro-interactions on hover/press; consistent image treatment.

## 3. Report format
```
## Summary  (direction as perceived, overall score 1–10, top 3 issues)
## Metrics  (colors N, font sizes N, radii N, shadows N, arbitrary values N, contrast failures N)
## P0 / P1 / P2  — each item: what, where (file:line or screen area), why it matters, concrete fix
## Suggested next step  (e.g. "consolidate tokens, then redesign hero")
```
