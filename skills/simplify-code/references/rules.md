# Simplification Rules

Each rule: **what**, *why*, and **don't** (when not to apply). Always behavior-preserving. These are **guidelines, not laws** — apply judgment; balance Clean Code's small-function preference with Ousterhout's deep modules.

## Contents
1. Naming · 2. Functions · 3. Deep modules & information hiding · 4. Types, states & boundaries · 5. Nesting & guard clauses · 6. Duplication (DRY) · 7. Dead code & comments · 8. Magic numbers/strings · 9. Conditionals · 10. Error handling & resources · 11. Objects, classes & SOLID · 12. Async & concurrency · 13. Formatting, layout & file budgets · 14. Tests · 15. Boy Scout, practicality & measurement · 16. Named Fowler smells · 17. Object Calisthenics (optional) · 18. AI-era / agent token cost · 19. Smell → refactoring map

## 1. Naming
- Use intention-revealing, pronounceable names: `elapsedDays` not `d`, `yyyymmdstr`.
- One word per concept: don't mix `fetch/get/retrieve` for the same operation.
- Searchable names: name constants instead of literals (see §8).
- Explanatory variables: name intermediate results of complex expressions (e.g., regex groups).
- No mental mapping: avoid single letters except tiny-scope loop indices/lambdas.
- No redundant context: `car.color` not `car.carColor`; no Hungarian notation or type suffixes when types exist.
- Functions are verbs (`sendInvoice`), booleans read as predicates (`isActive`, `hasItems`), classes are nouns.
- Name length proportional to scope size.
- **Clarity at the call site**: prefer APIs that read well where they are *used* (`users.filter(active)` beats cryptic flags).
- **Don't** rename public API symbols without asking.

## 2. Functions
- Small enough to do **one thing**; if you can extract a function with a meaningful name *and* the result is deeper (see §3), extract — don't create shallow one-liners that only add hops.
- One level of abstraction per function (**stepdown / newspaper rule**: high-level reads like a story, details below).
- Arguments: 0–2 ideal, 3 suspect. Group related args into a parameter object / data class / options struct.
- **No flag arguments / boolean soup**: `render(true)` → `renderForPrint()` / `renderForScreen()`. Multiple booleans in a signature are a smell.
- **No adjacent same-type parameters** that can be silently swapped (`copy(src, dst)` with two strings) — use named params, distinct types, or a parameter object.
- Prefer **return values over out-params** / mutable output arguments.
- No hidden side effects: don't mutate inputs or globals unexpectedly; return new values (clone) or name the side effect explicitly (`saveAndNotify`). Centralize real side effects (I/O) in few places.
- Command–query separation: a function either changes state or returns information, not both.
- **Mutating / nonmutating pairs**: name them clearly (`sort` vs `sorted`, `reverse!` vs `reversed`); don't surprise callers.
- Prefer default parameters over `x = x || default` short-circuits (beware falsy values like 0/"").
- Don't write to global/built-in prototypes.
- Prefer declarative/functional pipelines (map/filter/reduce, comprehensions) when clearer than loops — **don't** force them when a loop is more readable or performance-critical.
- Callers above callees; keep related functions close.

## 3. Deep modules & information hiding (Ousterhout)
- Prefer **deep modules**: a simple interface that hides a rich implementation. Complexity belongs *inside*; the API stays small and stable.
- **Information hiding**: don't leak internal data structures, temporary files, or error-recovery details through the public surface.
- Avoid **shallow modules** — classes/functions whose interface is nearly as complex as their implementation (thin wrappers that only add hops).
- Counterweight to "extract everything": many tiny functions with leaky signatures increase cognitive load and call-graph noise. Extract when it deepens the abstraction, not when it only shortens a file.
- **Don't** merge unrelated concerns into one "deep" god module — depth ≠ dumping ground (see SRP / god class).

## 4. Types, states & boundaries
- **Make illegal states unrepresentable**: use enums, newtypes, discriminated unions, and refined types so invalid combinations won't compile or construct (`StandingOrder` can't be `cancelled` and `shipped`).
- **Wrap meaningful primitives** (primitive obsession): `Email`, `UserId`, `Money` instead of bare `string`/`int` when domain meaning matters.
- **First-class collections**: a collection with domain behavior often deserves its own type (`OrderLines`, not `List<OrderLine>` passed everywhere with ad-hoc helpers).
- **Immutability at boundaries**: prefer immutable inputs/outputs across module and process borders; mutate only inside well-marked scopes.
- State **preconditions and postconditions** at public APIs (assertions, type contracts, doc contracts, or validation). Fail fast when violated.
- Prefer Options/Maybe/nullable annotations over sentinel values.

## 5. Nesting & guard clauses
- Replace nested conditionals with **guard clauses / early returns** for invalid or special cases.
- Invert `if (ok) { long } else { return err }` → `if (!ok) return err; long`.
- Use `continue`/`break` early in loops instead of wrapping the body.
- Extract deep inner blocks into named functions.
- Merge consecutive conditions with same outcome (`if a: return x; if b: return x` → `if a or b`) when it stays readable.
- Target ≤2 levels of indentation inside a function.
- **Don't** change evaluation order of conditions with side effects.

## 6. Duplication (DRY)
- Every piece of knowledge has a single authoritative representation (Pragmatic Programmer).
- Extract shared logic to functions/modules; parameterize differences.
- Rule of three: tolerate the second copy, abstract on the third — or earlier if the duplicated *concept* is clearly the same.
- **Don't** merge code that only looks alike but changes for different reasons (wrong abstraction is costlier than duplication).

## 7. Dead code & comments
- **Delete dead code**: unused functions, variables, imports, unreachable branches, obsolete feature flags (verify via search/IDE; beware reflection, dynamic dispatch, public exports).
- Delete commented-out code — version control remembers.
- Delete journal comments (change logs), noise comments that restate code, and positional/banner markers.
- Prefer self-explanatory code over comments; keep comments that explain **why**, business rules, non-obvious constraints, legal/TODO with ticket refs, public API docs.
- Update or remove misleading comments.

## 8. Magic numbers & strings
- Replace literals with named constants/enums: `86400000` → `MILLISECONDS_PER_DAY`.
- Group related constants in an enum/namespace.
- Obvious values (0, 1, empty string in natural use) may stay inline.

## 9. Conditionals
- Encapsulate complex predicates: `if (isEligibleForDiscount(order))`.
- Avoid negative names and double negatives: `isEnabled` over `isNotDisabled`; `if (!isNotX)` → `if (isX)`.
- Prefer positive branch first when both branches are significant.
- Replace repeated type/enum switches with **polymorphism**, strategy objects, or a lookup table/map.
- Avoid type checking for dispatch; rely on interfaces/duck typing (or static types in TS etc.).
- Use language null-safety operators (`?.`, `??`, `?:`, safe navigation) to collapse null checks — only if semantics match (null vs undefined vs falsy).
- Return boolean expressions directly: `if (c) return true; else return false;` → `return c;`.
- Use identity/strict comparison where the language has loose equality (`===`, PHP `===`).
- Ternaries for simple value selection only; never nest them.

## 10. Error handling & resources
- **Errors never silent**: never swallow (`catch {}` / `except: pass`); at minimum log with context or rethrow.
- Fail fast: validate inputs at boundaries.
- Use exceptions / Result / Either types instead of error codes and sentinel values.
- Provide context in exceptions (what failed, with which input); use specific exception types; wrap with cause chains (`raise ... from`, `%w`, `InnerException`).
- Don't return `null` when an empty collection, Optional, or Null Object fits; don't pass `null`.
- Keep try blocks small; extract the body of a try into its own function.
- Handle every promise rejection / future error.
- **Hidden limits are behavior:** when replacing a fixed buffer/array with a growable container (`std::string`, `Vec`, list), the old size limit and its error are observable. Keep them or ask.
- These are targets for new code; in a refactor, **golden rules 1–2 win**: don't convert error codes/strings/panics/null returns to exceptions/Result on a public API without approval.
- **RAII / `with` / `using` / `defer` / try-with-resources**: acquire and release resources via language constructs so cleanup runs on every path (success, error, early return).

## 11. Objects, classes & SOLID
- **SRP**: one reason to change per class/module. Split god classes/files.
- **OCP**: extend via new types/strategies, not by editing switch statements everywhere.
- **LSP**: subtypes must be substitutable; if overriding throws or ignores behavior, rethink the hierarchy.
- **ISP**: small, client-specific interfaces.
- **DIP**: depend on abstractions; inject dependencies instead of constructing them internally.
- Composition over inheritance; inheritance only for true "is-a".
- Encapsulate state: private fields, expose behavior not data ("tell, don't ask"); Law of Demeter (avoid `a.b().c().d()` train wrecks).
- Prefer immutability and value objects; avoid Singletons/global mutable state.
- Prefer `final`/sealed classes by default where the language supports it (PHP, Java, C#, Kotlin).
- Small classes, high cohesion, low coupling — but still **deep** (simple interface).

## 12. Async & concurrency
- Callbacks → promises → async/await to flatten "pyramids of doom".
- Flatten `.then()` chains; one `try/catch` around the awaited sequence.
- Run independent awaits concurrently (`Promise.all`, `asyncio.gather`) **only** when order/side effects don't matter — otherwise this is a behavior change.
- Don't mix sync blocking calls in async code; don't fire-and-forget without error handling.
- Keep shared mutable state out of concurrent code; prefer message passing/immutable data.

## 13. Formatting, layout & file budgets
- **Formatter is law**: use the project's formatter/linter (Prettier, ESLint, Black/Ruff, gofmt, rustfmt, clang-format, PHP-CS-Fixer/PSR-12, RuboCop, dotnet format, google-java-format, ktlint, SwiftFormat). Don't bikeshed style the tool already owns.
- Consistent capitalization per language convention.
- Vertical ordering: high-level first, helpers below; related code close; variables declared near use (stepdown/newspaper).
- Reasonable line length; one statement per line.
- **File length budgets**: prefer cohesive modules over multi-thousand-line files. When a file grows past local norms (~300–500 LOC as a soft smell, project-dependent), look for extractable deep modules — also reduces agent/context token cost.
- Don't reformat untouched files — it bloats diffs.

## 14. Tests
- Add **characterization tests** before refactoring untested code.
- One concept per test; tests are code too — apply the same rules.
- **FIRST**: Fast, Independent, Repeatable, Self-validating, Timely.
- Prefer testing observable behavior over private implementation details.

## 15. Boy Scout, practicality & measurement
- **Boy Scout rule / opportunistic refactoring**: leave touched code a little cleaner than you found it — within the agreed scope; don't expand into drive-by rewrites of unrelated modules.
- **Practicality beats purity**: a slightly imperfect but clear solution wins over a "perfect" pattern that obscures intent.
- **Measure before micro-optimizing**: don't "simplify" by inventing clever bit-twiddling or caching unless profiling shows a hotspot; readability first.
- Prefer deleting complexity over configuring it.

## 16. Named Fowler smells (watch for these)
| Smell | What it looks like | Typical move |
|---|---|---|
| **Feature envy** | Method uses another object's data more than its own | Move Method / Extract |
| **Data clumps** | Same group of fields/params travel together | Introduce Parameter Object / Extract Class |
| **Shotgun surgery** | One change requires edits in many places | Move/Inline to gather the knowledge |
| **Divergent change** | One module changes for many unrelated reasons | Split Module (SRP) |
| **God class / god file** | Huge type that knows/does everything | Extract Classes, deepen responsibilities |
| **Primitive obsession** | Bare strings/ints for domain concepts | Introduce Value Object / Enum / newtype |
| **Temporal coupling** | Methods must be called in a hidden order (`init` then `run`) | Combine into one call, builder, or type state |
| Long function / long param list / duplicate code / switch on type / speculative generality / dead code | (classic) | See §19 map |

Also watch: **message chains** (Demeter), **middle man**, **refused bequest**, **alternative classes with different interfaces**, **incomplete library class**.

## 17. Object Calisthenics (optional, use judiciously)
Jeff Bay's nine constraints — useful as short training drills or local strictness, **not** as project-wide dogma:
1. One level of indentation per method
2. Don't use the `else` keyword
3. Wrap all primitives and strings
4. First-class collections
5. One dot per line
6. Don't abbreviate
7. Keep entities small (≈50 lines / ≈10 files per package — treat numbers as soft)
8. No classes with more than two instance variables
9. No getters/setters/properties (tell, don't ask)

**Use judiciously**: applying all nine everywhere fights idiomatic code in many languages and can create shallow wrapper spam. Pick 1–3 constraints that address actual local pain.

## 18. AI-era / agent token cost (not AI-only)
Clean, modular code helps **humans and coding agents**:
- Clear names, deep modules, and short files reduce the context an agent must load — **refactoring can cut agent token cost** (Fowler: economic benefit of refactoring).
- Agents still need design judgment; AI lowers syntax cost, not the need for good abstractions (Uncle Bob).
- This skill remains primarily about human maintainability; agent efficiency is a bonus, not the sole goal. Do not "optimize for the LLM" at the expense of idiomatic, readable code.

## 19. Smell → refactoring map
| Smell | Refactoring |
|---|---|
| Long function | Extract Function, Decompose Conditional |
| Deep nesting | Guard Clauses, Extract Function |
| Long parameter list | Introduce Parameter Object, Preserve Whole Object |
| Flag argument / boolean soup | Split into separate functions |
| Adjacent same-type params | Named params, distinct types, parameter object |
| Duplicate code | Extract Function/Module, Pull Up Method |
| Switch on type | Replace Conditional with Polymorphism / Lookup Table |
| Magic number | Replace with Named Constant |
| Primitive obsession | Introduce Value Object / Enum / newtype |
| Feature envy | Move Method |
| Data clumps | Extract Class / Parameter Object |
| Shotgun surgery | Move related knowledge together |
| Divergent change | Split Module |
| God class/file | Extract Class, deepen interfaces |
| Temporal coupling | Merge steps, type-state, builder |
| Temporary variable reused | Split Variable, Inline Temp |
| Speculative generality | Inline/Collapse Hierarchy, remove unused params |
| Dead code | Delete |
| Comments explaining what | Rename / Extract Function |
| Callback hell | Promises / async-await |
| Shallow module sprawl | Inline and deepen |
| Silent catch | Log+rethrow, Result, or handle meaningfully |
| Manual resource cleanup | RAII / with / using / defer |

## 20. Smallest readable equivalent

Minimize concepts, duplicated knowledge, nesting, and indirection before counting lines. A clear loop can be smaller cognitively than a chained reduce; a named intermediate can be better than a dense expression.

1. Record the existing contract, including side effects, error text, ordering, empty/null/falsy values, ownership, and resource lifetime.
2. Delete proven dead private code and noise comments; check dynamic registration/reflection and import-time effects before removing symbols or imports.
3. Inline single-use aliases and pass-through private helpers only when they add no domain meaning or useful boundary. Don't repeat expensive or effectful expressions by inlining.
4. Replace mechanical code with an existing standard-library operation when its semantics match. Don't add a dependency for a shorter expression.
5. Merge equal branches or repeated predicates while preserving short-circuit evaluation and branch priority. Return a predicate directly only when the old result was already boolean (coercion matters).
6. Consolidate genuinely shared knowledge into one cohesive helper; retain small duplication when abstraction needs flags, callbacks, or many parameters to accommodate unrelated cases.
7. Reuse language resource constructs only when acquisition, cleanup timing/order, and exception behavior remain identical. Keep deliberate limits and compatibility wrappers.
8. Compare the diff: fewer moving parts, easier call sites, no new concerns, and no public API change. Stop when further shortening makes debugging or reading harder.

Examples of candidates, not automatic rewrites:
- Boolean branches: `if (predicate) return true; return false;` → `return predicate;` for a boolean predicate.
- Repeated effect-free outcomes: `if (a) return x; if (b) return x;` → `if (a || b) return x;` with the same short-circuit behavior.
- Copy-then-transform loops → the language's simple map/comprehension, preserving eager/lazy behavior, collection type, holes, order, and callback count.
- Getter/wrapper chains → the underlying private operation when wrappers provide no validation, tracing, ownership, or extension contract.

Read the language-specific redundancy notes before applying these patterns. Preserve explicit code whenever equivalence is uncertain.
