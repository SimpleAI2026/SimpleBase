# Language Notes (index)

General rules live in `rules.md`. Per-language notes give the **style authority**, **common redundancy → smallest readable replacement**, and **⚠ equivalence conditions**.

| Tier | File | Languages |
|---|---|---|
| 1 – mainstream | [`languages-tier1.md`](languages-tier1.md) | TypeScript, JavaScript, Python, SQL, HTML/CSS, Bash/Shell, Java, C#, C++, C, Go, Rust, Kotlin, Swift, PHP, PowerShell |
| 2 – established | [`languages-tier2.md`](languages-tier2.md) | Ruby, Dart, R, Scala, Lua, Objective-C, Assembly, Perl, MATLAB, Julia, Zig, Elixir, Haskell, Groovy, VB/VBA, Fortran, GDScript, Nix |

**How to use:** read only the matching section. Mixed files (e.g. Vue/JSX = TS/JS + HTML/CSS; SQL embedded in Python) → read each relevant section.

**Unlisted language:** apply `rules.md` + the project's formatter/linter; don't claim language-specific expertise. The closest relative is a hint, not a rule (Clojure ≈ functional idioms, T-SQL ≈ SQL plus dialect checks).

**Equivalence gate:** every replacement is valid only if outputs, side effects, evaluation order/count, errors, resource lifetime, and public APIs stay the same, and the project's language version supports the syntax. Fewer lines is never sufficient evidence.
