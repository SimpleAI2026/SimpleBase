# Token System (3 tiers)

```
Primitive  → raw values, named by what they ARE      (--blue-600, --space-4, --radius-2)
Semantic   → named by PURPOSE, reference primitives  (--color-accent, --color-text-muted, --space-section)
Component  → named by COMPONENT, reference semantics (--button-bg, --card-padding)
```

Rules:
- Components consume **semantic or component** tokens only, never primitives or raw literals.
- Theming (dark mode, brand variants) = swapping the semantic layer; primitives and components stay.
- Keep primitive sets small. If a value isn't on the scale, question the design before adding it.

## Example (CSS variables)
```css
:root {
  /* primitive */
  --neutral-0:#fff; --neutral-50:#f7f6f3; --neutral-200:#e7e5e0; --neutral-600:#6b6a66; --neutral-900:#1c1b19;
  --brand-500:#d9480f; --brand-600:#b93d0c;
  --space-1:4px; --space-2:8px; --space-3:12px; --space-4:16px; --space-6:24px; --space-8:32px; --space-12:48px; --space-16:64px;
  --radius-1:4px; --radius-2:8px; --radius-full:9999px;
  --font-sans:"Söhne","Helvetica Neue",system-ui,sans-serif; --font-serif:"Newsreader",Georgia,serif;
  --text-sm:14px; --text-base:16px; --text-lg:20px; --text-xl:32px; --text-2xl:48px;
  --ease-out:cubic-bezier(.2,.8,.2,1); --dur-fast:120ms; --dur-base:220ms;

  /* semantic */
  --color-bg:var(--neutral-0); --color-surface:var(--neutral-50); --color-border:var(--neutral-200);
  --color-text:var(--neutral-900); --color-text-muted:var(--neutral-600);
  --color-accent:var(--brand-500); --color-accent-hover:var(--brand-600);
  --space-inline:var(--space-3); --space-stack:var(--space-4); --space-section:var(--space-16);
  --shadow-raised:0 1px 2px rgb(28 27 25 / .06), 0 2px 8px rgb(28 27 25 / .04);

  /* component */
  --button-bg:var(--color-accent); --button-bg-hover:var(--color-accent-hover); --button-radius:var(--radius-1);
  --button-pad:var(--space-2) var(--space-4);
  --card-bg:var(--color-surface); --card-radius:var(--radius-2); --card-pad:var(--space-6);
}
[data-theme="dark"] {
  --color-bg:#121110; --color-surface:#1c1b19; --color-border:#2c2a27;
  --color-text:#ecebe8; --color-text-muted:#a3a19c; --color-accent:#f76b2b;
}
```

## Tailwind / other stacks
Map the semantic layer into `theme.extend` (colors, spacing, borderRadius, fontSize) or the framework's theme object; avoid arbitrary values (`text-[13px]`, `#3b82f6`) in markup.

## Documenting
For a design system, keep a short table per tier: token, value, usage, do/don't. Component specs list: anatomy, variants, sizes, states, tokens used, a11y notes.
