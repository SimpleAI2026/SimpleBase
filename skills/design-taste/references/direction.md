# Design Direction

## Contents
1. Brief questions  2. Tone catalog  3. Choosing type  4. Choosing palette  5. Writing the direction

## 1. Brief questions
- What is the single primary action on this screen?
- Who uses it, how often, on what device? (daily tool → density and speed; one-time visit → impression and clarity)
- What must feel *true* about the brand (trustworthy, fast, crafted, fun)?
- What exists already (logo, colors, fonts, component library)? Reuse before inventing.

## 2. Tone catalog (pick one, then commit)
| Tone | Feels like | Typical moves |
|---|---|---|
| Editorial | magazine, essay | serif display + quiet sans, wide margins, strong rules, few colors |
| Swiss / functional | signage, specs | grotesk, strict grid, flush-left, one signal color |
| Technical / dense | terminal, trading | mono accents, tight rows, tabular numbers, dark or neutral UI |
| Soft / organic | wellness, consumer | rounded forms, warm neutrals, gentle motion |
| Luxury / quiet | fashion, hospitality | high-contrast serif, lots of space, near-monochrome |
| Brutalist / raw | art, indie | system fonts, hard borders, visible structure, loud color blocks |
| Playful | kids, games, creator tools | chunky type, saturated palette, springy motion (sparingly) |
| Retro / industrial | hardware, archives | condensed type, muted inks, texture, labels |

Mixing two tones is fine only if one clearly leads.

## 3. Choosing type
- Pair by **contrast**, not similarity: display serif + neutral sans; grotesk + mono; condensed + humanist.
- Body text: highly legible, 16px+ on web, line-height 1.5–1.7, 60–75 characters per line.
- Headings: tighter line-height (1.05–1.25), slightly negative tracking at large sizes.
- Avoid reflexive defaults (Inter, Roboto, Arial, Space Grotesk everywhere). Choose with intent; system stacks are a valid *intentional* choice for brutalist/utility looks.
- Use tabular numerals for data.

## 4. Choosing palette
- Start from neutrals (tinted toward warm or cool, not pure gray), then add **one** accent; add a second only with a job.
- Define semantic colors (success/warning/danger/info) separately from brand accent.
- Dark mode is a separate palette, not an inversion: lower saturation, raise surfaces with lighter tones instead of shadows.
- Check contrast for every text/background pair you actually use.

## 5. Writing the direction (template)
```
Direction: <tone> for <audience>, optimized for <primary action>.
Type: <display> / <body> (+ <mono>).  Palette: <neutral family> + <accent> (+ semantic set).
Signature: <one memorable move>.  Density: <airy|balanced|dense>.  Motion: <none|subtle|expressive>.
```
