# Tier 1 Languages

Per language: **Style** (authoritative tools/guides) · **Redundant → simpler** (common bloat and the smallest readable replacement) · **⚠ Keep equivalent** (conditions under which the replacement is *not* the same program). Read only the section you need. Every replacement assumes the project's language version supports it.

## Contents
TypeScript · JavaScript · Python · SQL · HTML/CSS · Bash/Shell · Java · C# · C++ · C · Go · Rust · Kotlin · Swift · PHP · PowerShell

## TypeScript
- **Style:** [clean-code-typescript](https://github.com/labs42io/clean-code-typescript), [Google TS Style](https://google.github.io/styleguide/tsguide.html), ESLint + Prettier, project `tsconfig` strictness.
- **Redundant → simpler:**
  - `a && a.b && a.b.c` → `a?.b?.c` (when the result is only used as value-or-undefined).
  - `x !== null && x !== undefined ? x : d` → `x ?? d`.
  - Type annotations the compiler already infers on locals (`const n: number = 1`) → drop; keep annotations on exported/public signatures.
  - Parallel `if (kind === 'a') … else if (kind === 'b')` over a union → `switch` with `never` exhaustiveness check, or a `Record<Kind, …>` lookup.
  - Repeated inline object shapes → one named `interface`/`type` (follow project preference).
  - `as` casts that only silence a type the code already narrows → remove; `any` → `unknown` + narrowing only in private code.
- **⚠ Keep equivalent:** `a && a.b` returns `a` itself when falsy (`0`, `''`), `?.` returns `undefined`; `??` keeps `0`/`''`/`false`, `||` does not; removing `any` or adding generics must be re-checked with `tsc` (e.g. indexing `Record<string, T>` with a generic `K` fails with TS2536 under `--strict`); returning a `Map` instead of a plain object is an API change; removing a cast can change overload resolution; exported types, `enum` values (numeric vs string), optional vs `| undefined` under `exactOptionalPropertyTypes` are API. JavaScript traps (`for..in`, `sort()`, `Promise.all`) apply too.

## JavaScript
- **Style:** [clean-code-javascript](https://github.com/ryanmcdermott/clean-code-javascript), [Google JS Style](https://google.github.io/styleguide/jsguide.html), ESLint + Prettier.
- **Redundant → simpler:**
  - Push-in-loop building a new array → `map`/`filter` (when the loop has no early exit or side effects).
  - `if (cond) { return true; } return false;` → `return cond;` only if `cond` is already boolean; otherwise `return Boolean(cond)` or keep.
  - `function (x) { return f(x); }` passed as callback → `f` only if `f` ignores extra args and doesn't use `this`.
  - Manual object copy/merge loops → spread `{ ...a, ...b }` (shallow, own enumerable props).
  - `.then()` ladders → `async`/`await` inside private code with one `try/catch`.
  - `arr.indexOf(x) !== -1` → `arr.includes(x)` (except NaN, see below).
- **⚠ Keep equivalent:** `['1','2'].map(parseInt)` is the classic extra-args bug; `forEach(async …)` doesn't await; `map` preserves holes, loop+push writes `undefined`; `includes` finds `NaN`, `indexOf` doesn't; spread drops prototypes/getters; `sort`/`reverse` mutate and default `sort()` compares as strings (`[10, 9, 1]` → `[1, 10, 9]`); `for..in` → `Object.keys` drops inherited enumerable keys; sequential `await` loop → `Promise.all` runs concurrently and doesn't stop at the first failure; `var`→`let` changes hoisting/closure capture; `==`→`===` changes `null == undefined` and string/number comparison.

## Python
- **Style:** [PEP 8](https://peps.python.org/pep-0008/), [PEP 20](https://peps.python.org/pep-0020/), [clean-code-python](https://github.com/zedr/clean-code-python), Black/Ruff.
- **Redundant → simpler:**
  - Append loop → list/dict/set comprehension (keep it to one `for` + one `if`).
  - `for i in range(len(xs)): x = xs[i]` → `for x in xs` / `for i, x in enumerate(xs)`; parallel indexing → `zip`.
  - `if k in d: v = d[k] else: v = default` → `d.get(k, default)` when `default` is cheap and side-effect free.
  - Counting/grouping dict boilerplate → `collections.Counter` / `defaultdict`.
  - `open()` + `close()` → `with`; manual path string joins → `pathlib` only where the project already uses it.
  - `if x == True` / `len(xs) == 0` → see ⚠; `return True if c else False` → `return c` when `c` is bool.
  - Getter/setter methods with no logic → plain attribute (private classes only).
- **⚠ Keep equivalent:** list → generator makes it single-pass and lazy; `d.get(k, f())` evaluates `f()` every time, and `d[k]` raised `KeyError`; `if xs:` treats `None`, `0`, `''` alike; `x == True` vs truthiness (`2`, `'yes'`); mutable default args; closures capture loop variables late; `%` formatting vs f-strings (`'%d' % 3.7`); `sorted` is stable, `max` returns the first max.

## SQL
- **Style:** project dialect + SQLFluff/sqlfmt conventions; consistent keyword case; meaningful aliases.
- **Redundant → simpler:**
  - Same subquery repeated → one CTE; deeply nested derived tables → named CTEs in reading order.
  - `CASE WHEN x IS NULL THEN d ELSE x END` → `COALESCE(x, d)`.
  - `WHERE a = 1 OR a = 2 OR a = 3` → `a IN (1, 2, 3)` (non-NULL literals).
  - `SELECT DISTINCT` hiding a fan-out join → fix the join (only if rows are provably identical).
  - Implicit comma joins → explicit `JOIN … ON` preserving the same predicates.
  - Self-join for "previous row" → window `LAG`/`ROW_NUMBER` when ties and ordering are defined identically.
- **⚠ Keep equivalent:** `NOT IN (subquery)` returns nothing if the subquery yields a NULL; `UNION` ≠ `UNION ALL`; predicate moved from `ON` to `WHERE` turns LEFT into INNER join; `COUNT(*)` vs `COUNT(col)`; CTE materialization varies (Postgres <12 always materialized); `AND`/`OR` precedence; volatile functions (`random()`, `now()`) evaluated once vs per row; never build SQL by concatenation.

## HTML / CSS
- **Style:** [Google HTML/CSS Style](https://google.github.io/styleguide/htmlcssguide.html), Prettier, Stylelint, project design tokens/utility framework.
- **Redundant → simpler:**
  - Wrapper `div`s with no styling, semantics, or hooks → remove.
  - Repeated declarations across selectors → shared class or existing token/custom property.
  - Longhands → shorthand (`margin: 0 auto`) when all longhands are set.
  - `div onclick` → `button` (also an accessibility fix — report it).
  - Duplicate media queries → merge at the same cascade position.
- **⚠ Keep equivalent:** wrappers may carry flex/grid context, `id`/class hooks used by JS/tests, or event delegation; shorthand resets unspecified longhands; selector specificity and source order decide the winner; `!important` removal can restyle; changing element type changes default styles, form submission (`button` defaults to `type="submit"`), and keyboard behavior.

## Bash / Shell
- **Style:** [Google Shell Style](https://google.github.io/styleguide/shellguide.html), ShellCheck, shfmt. Know whether the script is POSIX `sh` or Bash.
- **Redundant → simpler:**
  - `cat file | grep x` → `grep x file`; `echo $(cmd)` → `cmd`.
  - Backticks → `$(…)`; `[ ]` → `[[ ]]` in Bash only.
  - `$(basename "$p")` / `$(dirname "$p")` → `${p##*/}` / `${p%/*}` only for paths without trailing slash and with a directory part.
  - Repeated command blocks → a function; growing argument strings → arrays (Bash).
  - `if cmd; then true; else false; fi` / `cmd; if [ $? -ne 0 ]` → `if ! cmd; then …`.
- **⚠ Keep equivalent:** quoting changes word splitting/globbing (usually a fix — report it); `cat f | while read` runs in a subshell (variables don't escape); `$(…)` strips trailing newlines; adding `set -euo pipefail` is a behavior change, not cleanup; `basename` vs `${p##*/}` differ for `dir/` and `/`; `[[` is not POSIX.

## Java
- **Style:** [Google Java Style](https://google.github.io/styleguide/javaguide.html), [clean-code-java](https://github.com/leonardolemie/clean-code-java), google-java-format, project Java version.
- **Redundant → simpler:**
  - Data-carrier classes with boilerplate getters/equals/hashCode → `record` (16+, if not a public mutable bean).
  - Loop+add into a new list → simple stream (`filter`/`map`/`toList()`), or keep the loop if it has breaks/exceptions.
  - `try { … } finally { r.close(); }` → try-with-resources.
  - `if (x != null) { return x; } return d;` → `Objects.requireNonNullElse(x, d)` (Java 9+) or keep.
  - Explicit types on obvious locals → `var` (10+) where the project uses it.
  - Anonymous single-method classes → lambdas/method refs (watch `this`).
- **⚠ Keep equivalent:** `Stream.toList()` is unmodifiable, `Collectors.toList()` is not guaranteed either way, a hand-built `ArrayList` is mutable; `put` loop → `Collectors.toMap` throws `IllegalStateException` on duplicate keys (add a merge fn) and `NullPointerException` on null values (merge fn can't fix it), and the map type is unspecified; `record` changes `equals`/accessor names (`x()` not `getX()`) — API; streams are lazy and single-use; try-with-resources closes in reverse order and adds suppressed exceptions; `requireNonNullElse` throws if `d` is null; `==` vs `.equals`, autoboxing NPEs.

## C#
- **Style:** [.NET conventions](https://learn.microsoft.com/dotnet/csharp/fundamentals/coding-style/coding-conventions), [clean-code-dotnet](https://github.com/thangchung/clean-code-dotnet), `dotnet format`, `.editorconfig`.
- **Redundant → simpler:**
  - `x != null ? x.Y : null` → `x?.Y`; `x != null ? x : d` → `x ?? d`; `if (x == null) x = d;` → `x ??= d`.
  - Backing field + trivial get/set → auto-property.
  - `using (…) { … }` wrapping a whole method → `using var` declaration.
  - Hand-written filter/projection loops → LINQ in non-hot paths.
  - Repeated `switch` statements returning values → switch expression with patterns.
  - DTO classes with only properties + equality → `record` (if equality semantics are wanted).
- **⚠ Keep equivalent:** LINQ is deferred — multiple enumeration re-runs queries/side effects; `using var` disposes at scope end, later than a block; `throw;` vs `throw ex;`; switch expressions throw on no match where a statement fell through; `record` gives value equality (behavior change for collections/dictionaries); `?.` on a value-returning member makes it nullable.

## C++
- **Style:** [C++ Core Guidelines](https://isocpp.github.io/CppCoreGuidelines/), [Google C++ Style](https://google.github.io/styleguide/cppguide.html), clang-format/clang-tidy, project standard (C++11/17/20).
- **Redundant → simpler:**
  - `new`/`delete` pairs in a scope → `std::unique_ptr`/`make_unique` or a stack object.
  - Index loops over containers → range-for; hand-written find/count/any/accumulate → `<algorithm>`.
  - `std::map::find` + `!= end()` just to test → `contains` (C++20) / `count`.
  - Explicit iterator types → `auto`.
  - `#define` constants/functions → `constexpr` variables/`inline` functions.
  - Out-params → return values / `std::optional` / structured bindings (private code).
  - Hand-written copy/move/destructor that only does member-wise work → rule of zero.
- **⚠ Keep equivalent:** iterator invalidation when the loop mutates the container; destruction order; `auto` may deduce a copy (`auto` vs `auto&`) or proxy (`vector<bool>`); removing `std::move` or adding one changes copies/moved-from state; fixed buffers → `std::string` loses size limits; macros may be used for `#if` or token pasting; ABI of public headers.

## C
- **Style:** project guide (Linux kernel, etc.), clang-format, `-Wall -Wextra`.
- **Redundant → simpler:**
  - Duplicated cleanup at every error return → single `goto cleanup` with labeled teardown (idiomatic; don't "simplify" it away).
  - Identical branch tails → hoist after the branch.
  - Magic numbers → `enum` or `static const`; function-like macros → `static inline` functions when no generic/token tricks are needed.
  - `if (p != NULL) free(p);` → `free(p);` (`free(NULL)` is a no-op).
  - Repeated `sizeof(type)` → `sizeof *ptr`.
- **⚠ Keep equivalent:** macros evaluate arguments multiple times — an inline function changes that (usually for the better, still report it); integer promotion and signed/unsigned compares; `errno` must be read before other calls; `volatile`/`restrict`; `strncpy` vs `snprintf` termination; evaluation order of side effects.

## Go
- **Style:** [Effective Go](https://go.dev/doc/effective_go), [Code Review Comments](https://go.dev/wiki/CodeReviewComments), [clean-go-article](https://github.com/Pungyeon/clean-go-article), gofmt/goimports/staticcheck.
- **Redundant → simpler:**
  - `if err == nil { …happy… } else { return err }` → `if err != nil { return err }` then happy path, left-aligned.
  - `if cond { return true }; return false` → `return cond`.
  - `else` after `return` → drop.
  - Manual min/max/contains/index loops → `min`/`max` builtins (1.21+), `slices.Contains`/`slices.Index`/`maps.Keys` (1.21+/1.23+) when the module's Go version allows.
  - Interfaces declared next to their only implementation → define at the consumer or remove.
  - `for i := 0; i < len(xs); i++` with only `xs[i]` reads → `for _, x := range xs`.
- **⚠ Keep equivalent:** `nil` vs empty slice/map (JSON `null` vs `[]`, `== nil` checks); typed-nil in an interface is non-nil; wrapping errors changes `Error()` text; `defer` runs at function exit, not loop iteration; map iteration is random; range copies values (mutations to `x` don't stick); loop-variable capture before Go 1.22.

## Rust
- **Style:** [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/), rustfmt, clippy (its suggestions are usually the minimal idiom).
- **Redundant → simpler:**
  - `match r { Ok(v) => v, Err(e) => return Err(e) }` → `r?` (when `From` conversion is identical or identity).
  - `match opt { Some(x) => Some(f(x)), None => None }` → `opt.map(f)`; `if let Some(x) = o { x } else { d }` → `o.unwrap_or(d)` / `unwrap_or_else`.
  - Index loops → iterators; push loops → `collect()`.
  - `.clone()` only to satisfy a borrow that can be shortened → borrow; `&String`/`&Vec<T>` params → `&str`/`&[T]` (private fns).
  - `return x;` at the end of a function → `x`.
  - Boolean flags in private APIs → enums.
- **⚠ Keep equivalent:** `unwrap()` → `?` turns a panic into an `Err` (API change); `?` may call `From::from` and change the error type; `unwrap_or(f())` evaluates eagerly; Drop order and lock-guard lifetimes; `as` casts truncate silently, `try_from` errors; iterator laziness (side effects in `map` without consumption never run).

## Kotlin
- **Style:** [Kotlin coding conventions](https://kotlinlang.org/docs/coding-conventions.html), ktlint/detekt.
- **Redundant → simpler:**
  - `if (x != null) x.y else null` → `x?.y`; `if (x != null) x else d` → `x ?: d`.
  - Java-style POJOs → `data class` (private/internal models).
  - `when` with repeated equal branches → combine conditions `a, b ->`.
  - Loop + mutable list → `map`/`filter`/`buildList`.
  - Explicit `get`/`set` with no logic → properties; `.let { it }` noise → remove.
  - Nested `apply`/`also`/`let`/`run` → plain locals or one scope function.
- **⚠ Keep equivalent:** `?:` right side may `return`/`throw` (control flow); `data class` adds `equals`/`copy`/`componentN` (public API); `Sequence` is lazy, `List` ops eager; `!!` removal moves the NPE site; default args and `@JvmOverloads` affect Java callers.

## Swift
- **Style:** [Swift API Design Guidelines](https://www.swift.org/documentation/api-design-guidelines/), [Google Swift Style](https://google.github.io/swift/), SwiftFormat/SwiftLint.
- **Redundant → simpler:**
  - Pyramids of `if let` → `guard let … else { return }`; `if let x = x` → `if let x` (Swift 5.7+).
  - Single-expression functions/closures → implicit return; `return` in single-expression getters → drop.
  - Loop + append → `map`/`filter`/`compactMap`.
  - `switch` cases with identical bodies → `case .a, .b:`.
  - Explicit `self.` where not required by capture rules → drop (follow project).
- **⚠ Keep equivalent:** `guard` must exit scope; `compactMap` drops nils that a loop might have kept; `class`↔`struct` changes identity/copy semantics; `try?` swallows errors; `weak`/`unowned` lifetimes; `lazy` collections re-evaluate closures.

## PHP
- **Style:** [PSR-12](https://www.php-fig.org/psr/psr-12/), [clean-code-php](https://github.com/piotrplenik/clean-code-php), PHP-CS-Fixer/PHP_CodeSniffer, project PHP version.
- **Redundant → simpler:**
  - `isset($a['k']) ? $a['k'] : $d` → `$a['k'] ?? $d` (exactly equivalent).
  - `$x !== null ? $x->y : null` → `$x?->y` (8.0+).
  - Constructor assigning each param to a property → constructor promotion (8.0+).
  - Value-returning `switch` → `match` only when every case already compares identical types.
  - `foreach` building a filtered/mapped array → keep or `array_map`/`array_filter` when keys don't matter.
  - Docblock types duplicated by native types → drop the redundant tags.
- **⚠ Keep equivalent:** `switch` uses loose `==`, `match` uses `===` and throws `UnhandledMatchError` on no match; `in_array`/`array_search` are loose by default (`in_array('1', [1])` is true) — adding the strict flag changes matches; `$x == true` is truthiness (`'0'` false, `'false'` true); `array_filter` preserves keys (wrap in `array_values` if a list was expected); `empty('0')` is true and `empty` suppresses undefined-index warnings; `isset` is false for null values while `array_key_exists` is true; arrays copy on write, objects are handles; references `&`. A user request for "strict everywhere" is a behavior change — apply only where types are already identical.

## PowerShell
- **Style:** [PowerShell Practice and Style](https://poshcode.gitbooks.io/powershell-practice-and-style/), PSScriptAnalyzer, approved `Verb-Noun`.
- **Redundant → simpler:**
  - Long repeated parameter lists → splatting `@params`.
  - Aliases in scripts (`gci`, `%`, `?`) → full cmdlet names (clearer, same behavior).
  - `foreach` building an array with `+=` → assign the loop output (`$r = foreach (…) { … }`).
  - Duplicated pipeline blocks → advanced function with `process {}`.
  - `if ($x -eq $null)` → `if ($null -eq $x)` (correct for arrays too — report as a fix).
- **⚠ Keep equivalent:** single-item output is unrolled to a scalar (use `,$x` / `@()` if the old code relied on an array); anything not captured goes to the output stream; `-ErrorAction` and `$ErrorActionPreference` scope; `SupportsShouldProcess`/`-WhatIf` contracts.
