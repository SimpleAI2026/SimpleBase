---
name: simplify-code
description: Simplify, clean up, and refactor existing code in any mainstream language (JS/TS, Python, Java, Go, Rust, C#, C/C++, PHP, SQL, Bash, and more) while preserving behavior and public APIs. Use whenever the user asks to simplify, tidy, refactor, de-nest, deduplicate, remove dead code, shorten code without changing behavior, polish recently written code, modernize syntax safely, or fix code smells (god class, feature envy, primitive obsession), or pastes code saying "make this cleaner" — including 简化代码, 精简代码, 重构, 代码重构, 优化代码可读性, 整理代码, 代码整洁, 去除重复代码, 减少嵌套, 代码坏味道, 这段代码太乱了. Not for performance tuning, bug fixes, new features, translating between languages, configuring linters/formatters, writing tests as the main task, general code review or security audits (read-only simplification assessments are supported), or prose editing (性能优化、修 bug、新功能、代码翻译、配置 lint、写单测、代码审查、文章精简).
license: MIT
---

# Simplify Code

Make code easier to read and change without changing what it does. The target is the *smallest readable equivalent*: fewer concepts, duplicated knowledge, nesting levels, and pointless hops — not the fewest characters. Small safe steps beat big rewrites, because each step can be checked on its own.

## Golden rules

Rules 1–2 outrank everything else in this skill, including the checklists in `references/`. Users ask for cleanup expecting to merge it without re-testing the world; a silent behavior change betrays that trust.

1. **Preserve behavior.** Keep outputs, side effects and their order, error types and messages, null/empty handling, laziness, resource lifetime, and complexity class. A change to any of these is a bug fix or redesign — propose it separately so the user can judge it on its own.
2. **Preserve public APIs.** Exported names, signatures, public fields, error text, wire/DB/CLI formats, and mutability contracts have callers you can't see. Callback→promise, sync→async, panic→`Result`, `null`→exception, parameter objects, and newtypes are API changes: propose, don't apply.
3. **Follow project conventions.** Read applicable `AGENTS.md` / `CLAUDE.md`, linter/formatter config, and neighbouring code; they beat generic advice here because consistency within a codebase matters more than any one style. Project lint/formatter rules also outrank this skill's stylistic preferences (including no nested ternaries): if de-nesting triggers max-depth or another lint rule, try a cohesive helper; otherwise retain the existing form — don't disable lint or rewrite its config. Let the formatter decide layout; hand-formatting creates diff noise that hides the real changes. Use only syntax the configured runtime/compiler supports; don't add dependencies or change config for cleanup.
4. **Prefer clarity over compression.** Avoid nested ternaries, dense one-liners, and chains that hide side effects; the next reader pays for every clever line. Keep helpful boundaries, intention-revealing names, and comments that explain *why*.
5. **Don't over-engineer.** Add abstractions only when they remove real duplication or complexity now; speculative ones are complexity you must maintain. Prefer deleting complexity to relocating it, and deep modules to many shallow helpers.
6. **Respect scope.** Touch the code the user named; if none, the code changed in this session or the working diff. Unrequested edits elsewhere inflate review cost and may collide with others' work, so list out-of-scope smells as suggestions instead.
7. **Report verification honestly.** Claim only checks you actually ran, with the command and result. If something wasn't run, say "not run" and why. A false "verified" leads the user to merge broken code — worse than admitting uncertainty.

## Choose the mode

- **Pasted snippet, no repo:** reason about equivalence directly; check edge inputs mentally, or run a tiny script if tools exist. Return the rewritten code plus notes, and say verification was by reasoning.
- **Read-only simplification assessment:** when asked to analyze, assess simplification opportunities, or report without edits, inspect and prioritize proposals only. Don't edit source, add tests, apply demonstration patches, create branches, commit, or push. Write report files only if requested. Use `references/assessment.md` for scope, priorities, and reporting; this is not a general correctness/security audit.
- **Repository code:** read callers and tests, edit in place, run the project's tests, linters, and type checker. Read `references/assessment.md` when the repository is large or verification is constrained.
- **Already simple:** say so and change nothing (or only trivially safe bits). Inventing churn wastes review time and risks regressions for no benefit.

## Workflow

For read-only assessments, steps 2/6 inspect existing test commands and available evidence; don't add tests or run commands that mutate the checkout. Step 5 produces proposals rather than edits. State the mode and sampled scope before reporting findings.

1. **Understand the contract.** In 1–2 sentences: inputs, outputs, side effects, errors, edge cases. Identify the public surface (`export`, `pub`, `public`, `__all__`, package entry points, callers in other modules).
2. **Secure a safety net.** Run existing tests. For risky repo changes without coverage, add small characterization tests first; if impossible, stay conservative and say so. Edge inputs worth checking: `0`, `''`, `false`, null/None/nil/undefined, NaN, negatives, empty and duplicate collections, duplicate keys, ties, unknown enum values, inherited properties, paths with spaces, size limits.
3. **Plan by payoff and risk.** Dead code and noise → guard clauses/nesting → redundant branches and temporaries → duplication → naming and magic values → conditionals → structure/classes. Use `references/rules.md` for the full catalogue and §20 for the minimum-code procedure.
4. **Apply language idioms.** Open `references/languages.md`, then read only the section for the language at hand in `languages-tier1.md` or `languages-tier2.md`. Each section lists common redundancy, the smaller replacement, and when it stops being equivalent.
5. **Refactor in small steps.** One behavior-preserving transformation at a time (delete, inline, extract, de-nest, merge branches, use a resource construct), so any failure points at a single step.
6. **Verify.** Re-run tests, linters, build, and the type checker — always after changing types (removing `any`, adding generics, narrowing nullability), since those break compilation in ways reasoning misses. Then review every rewritten line against `references/behavior-traps.md` and ask: *which concrete input makes old and new differ?* If one exists, revert only that step's own edits or flag it; never reset unrelated user changes. Compare failures/warnings against a pre-edit baseline. Missing/old tools, heavy builds, and untested platforms require explicitly partial verification, not a success claim; follow `references/assessment.md`.
7. **Report** in the user's language using the format below.

**User asked for a risky change?** (e.g. "replace `||` with `??`", "strict comparisons everywhere", "use `Promise.all`") Apply the safe parts. For each unsafe part, decline or ask, citing a concrete input whose result would change.

## Output format

```
Summary: <what was simplified>; behavior/API unchanged | changes needing approval listed below.
Changes:
- <rule/category>: <what and why, one line each>
Not changed: <tempting edits skipped and why (API, semantics, scope)>
Needs approval: <proposed behavior/API changes with the input that would differ>   (omit if none)
Bugs found (not fixed): <file:line, suspected/confirmed, reproduction or reasoning; keep separate from simplifications>   (omit if none)
Scope: <mode, files/modules sampled, revision if known, excluded areas; no repository-wide claim from a sample>
Verification: <each command actually run + result>; <what was not run and why>; <edge inputs checked by reasoning>
```

Then show the diff or the rewritten code. For a no-change result, state why the code is already minimal and stop. Keep it short.

## Quick checklist

Apply freely to private/local code; at public boundaries only propose.

- **Delete:** unused code/imports/params (beware reflection, dynamic registration, exports), commented-out code, journal and "what" comments.
- **De-nest:** guard clauses, early `continue`, inverted conditions; aim for ≤2 levels.
- **Collapse redundancy:** `if c return true else false` → `return c` (when `c` is boolean); merge branches with identical bodies; drop single-use temporaries and pass-through wrappers that add no meaning; replace hand-written stdlib operations only when semantics match exactly (check `toMap`, `Object.keys`, `sort`, `Promise.all` in the traps file).
- **Deduplicate** knowledge that changes together; tolerate look-alike code that changes for different reasons.
- **Name** intent; replace magic values with named constants.
- **Conditionals:** extract complex predicates; avoid double negatives; use lookup tables or polymorphism for repeated type switches (unknown-key behavior must match).
- **Resources/errors:** `with`/`using`/`defer`/RAII/try-with-resources when cleanup order and exceptions stay equivalent; never swallow errors that were previously raised.
- **Structure:** split god functions/classes along real responsibilities; inline shallow helpers; keep deep modules.

## References

| File | Read when |
|---|---|
| `references/languages.md` → `languages-tier1.md` / `languages-tier2.md` | Always, for the one language being edited (section only). |
| `references/behavior-traps.md` | Step 6, for every rewritten line. |
| `references/rules.md` | Planning non-trivial refactors; §20 for minimum-code decisions; §19 smell → refactoring map. |
| `references/examples.md` | Unsure whether a rewrite is safe; contains safe, propose-only, and reject/leave-alone examples plus report samples. |
| `references/sources.md` | User asks where a rule comes from. |
| `references/assessment.md` | Read-only simplification assessments, large-repository sampling, priority/risk ratings, lint conflicts, unavailable tools, and partial/platform verification. |
