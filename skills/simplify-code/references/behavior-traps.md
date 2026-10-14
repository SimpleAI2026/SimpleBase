# Behavior Traps — "simplifications" that silently change behavior

Use in workflow step 6 (Verify). For each rewritten line, ask: *which input makes old and new differ?* If one exists, keep the old semantics, or flag it and ask. These are the cases tests most often miss, so check each relevant one with a concrete input.

## Contents
Indexing & iteration · Sorting & ordering · Collections & maps · Formatting & strings · Truthiness, equality & defaults · Parsing · Async & ordering · Side effects & deduplication · Types & compilation · Errors & contracts · SQL · Shell · Numbers

## Indexing & iteration
- Loop with `items[i]` → `slice`/`substr`/`[a:b]`: negative, NaN, non-integer, or out-of-range bounds differ (JS `items[-1]` is `undefined`, `slice(-40)` counts from the end; Python lists wrap negatives too).
- `for` → `map`/`forEach`/comprehension: `async` callbacks aren't awaited; early `break`/`return` lost; holes in sparse arrays skipped by `map`/`forEach` but visited by an index loop.
- JS `for (k in obj)` → `Object.keys`/`Object.entries`: `for..in` also visits **inherited enumerable** keys (objects made with `Object.create(proto)`, class prototypes with assigned props, polluted prototypes). `Object.keys` only returns own keys. Keep `for..in`, or confirm inputs are plain object literals. Conversely, adding a `hasOwnProperty` filter is also a change.
- `for..in` over arrays yields **string** indices and extra props; `for..of` yields values — not interchangeable.
- Python iterating a dict while mutating it; Go `range` copies values; Java enhanced-for throws `ConcurrentModificationException` where an index loop didn't.

## Sorting & ordering
- **JS default `sort()` compares as strings**: `[10, 9, 1].sort()` → `[1, 10, 9]`. Replacing it with `(a, b) => a - b` "fixes" it but changes output; keep the default or flag it. `sort` and `reverse` mutate in place; `toSorted`/`slice().sort()` copy.
- `localeCompare` vs `<` comparison; case-sensitive vs case-insensitive order.
- Tie-breaking: Rust `max_by_key` returns the **last** max, Python `max` the **first**; stable (`sorted`, `Array.prototype.sort` since ES2019, Java `Collections.sort`) vs unstable (`sort_unstable`, C++ `std::sort`).
- Iteration order of hash maps/sets: Python dict and JS object/Map keep insertion order (JS puts integer-like keys first); Go maps, Java `HashMap`, Rust `HashMap` don't. Switching container types changes output order.

## Collections & maps
- **Java `Collectors.toMap`** replacing a `put` loop:
  - duplicate keys throw `IllegalStateException` (the loop kept the last value) → pass a merge function `(a, b) -> b`;
  - **null values throw `NullPointerException`** (`HashMap.merge` rejects null) — the merge function doesn't help; keep the loop or filter nulls deliberately;
  - the result type is unspecified (currently `HashMap`) → use the 4-arg overload with `LinkedHashMap::new`/`TreeMap::new` if order or type mattered.
- Java `Stream.toList()` is unmodifiable; `Collectors.toList()`/`toSet()` give no mutability guarantee; `List.of`/`Map.of` reject nulls and duplicates.
- Java `groupingBy` rejects null keys; `Collectors.joining` on nulls prints `"null"`.
- JS object → `Map`: different API (`get` vs `[]`), JSON serializes to `{}`, key types (`1` vs `'1'`) differ. A public return type change.
- Python `dict.get(k, default)` evaluates the default eagerly; `setdefault` mutates; `defaultdict` creates keys on read; `Counter` returns 0 for missing keys.
- PHP `array_filter`/`array_unique` preserve keys (JSON array becomes object); `array_merge` renumbers integer keys, `+` doesn't.
- Go `nil` vs empty slice/map (JSON `null` vs `[]`/`{}`); Rust `collect()` target type decides dedup/order (`Vec` vs `HashSet`).

## Formatting & strings
- `%d`/`%s` → f-string/template/`format`: `"%d" % 3.7` = `3`, `f"{3.7}"` = `3.7`; `True` → `1` vs `True`; `None` raises vs prints.
- `String.format("%.2f", double)` vs integer-cents arithmetic: rounding differs.
- `+` with a string operand concatenates (`'10' + '10'` = `'1010'`). Preserve the quirk or flag it as a separate bug.
- Template literals call `toString` differently from `+` for objects with `Symbol.toPrimitive`/`valueOf`.
- Locale-sensitive case changes (`toUpperCase` vs `toLocaleUpperCase`, Java `toLowerCase()` without `Locale.ROOT` — Turkish i).

## Truthiness, equality & defaults
- `x || d` → `x ?? d` / default params: `0`, `''`, `false`, `NaN` stop falling back (default params only replace `undefined`).
- `==` → `===`: String objects, `null == undefined`, number vs numeric string.
- Python `== True` → `if x:`: `"yes"`, `[1]`, `2` become true; `1`/`1.0` were already true. `if x is not None` vs `if x`: `{}`, `0`, `''` differ.
- **PHP loose → strict changes results:**
  - `in_array($x, $arr)` / `array_search` / `array_keys($a, $v)` are loose by default: `in_array('1', [1])`, `in_array('abc', [0])` (PHP < 8), `in_array(null, [0])` are true. Adding `true` as the strict flag changes matches unless types are already identical.
  - `$a == true` is truthiness: `'0'`, `''`, `[]`, `0`, `null` are false; `'false'` and `' '` are true. `=== true` matches only the boolean.
  - `empty($x)` treats `'0'`, `0`, `0.0`, `''`, `[]`, `null` as empty and suppresses undefined-index warnings; `$x === ''` or `!isset` are not equivalent replacements.
  - `switch` (loose) → `match` (strict, throws `UnhandledMatchError` with no default).
  - PHP 8 changed `0 == 'abc'` from true to false — know the runtime version.
- Ruby/Lua: only `nil`/`false` are falsy, so `x || d` keeps `0` and `''`.
- Removing `x != null &&` before a property access can change NaN/undefined handling.

## Parsing
- `split(sep)` → `split_once`/`partition`/`split(sep, 1)`: extra separators are now part of the value (`"a=1=2"`).
- `int()`/`parseInt`/`Number`/`Atoi` are not interchangeable (whitespace, `"1e3"`, `"0x10"`, trailing junk, empty string → `0` for `Number`).
- JS `['1','2','3'].map(parseInt)` → `[1, NaN, NaN]` (index passed as radix).

## Async & ordering
- Sequential `await` in a loop → `Promise.all(items.map(…))`: tasks start **concurrently** (order of side effects, DB writes, migrations changes), all run even after one fails (no stop at first failure), and the first rejection rejects the whole while others keep running. `Promise.allSettled` changes the result shape. Keep the loop when order or fail-fast matters; it is a performance change, out of this skill's scope.
- `asyncio.gather` behaves the same way; Python `return_exceptions` changes the result.
- Callback → promise/async, or sync → async, changes the API and timing.
- Removing an `await` before `return` inside `try` loses the catch for that rejection.

## Side effects & deduplication
- **Merging repeated effectful calls changes the effect count**: five `type cmd1 … cmd5` checks → one `type cmd1 … cmd5` keeps stdout on success but prints **one** stderr line instead of five when commands are missing; `$?`/`$?`-style per-command results are lost. Same for repeated `warn`/`log` calls, validators run per item, or counters incremented inside the expression.
- Extracting repeated code into a helper runs side effects **once instead of N times** — or N times instead of once if the helper is called at multiple sites. Count the executions before and after; deduplicate pure knowledge, not effectful calls whose count is observable.
- `if f(x); then …` → `f(x) && …` in shell: same side effect, but the `if` form returns 0 when `f` fails, the `&&` chain propagates the failure — script exit status changes.
- Caching/memoizing a lookup silently changes behavior for non-idempotent or volatile sources (time, random, network, config reload).

## Types & compilation
- Removing `any`, adding generics, or narrowing a type can **fail to compile** (`tsc --strict`: indexing `Record<string, T>` with a generic `K` → TS2536; `K extends keyof T` vs `string`). Always re-run the type checker after type changes — reasoning alone misses these.
- Tightening a public type is an API change for callers (stricter params) or for consumers (narrower returns can be fine; wider aren't).
- Java/C#/Kotlin: lambda/method-ref conversion changes overload resolution or checked-exception handling; `var` can infer a more specific type.
- Rust: `?` inserts `From::from`; changing `&String` → `&str` in public fns is an API change.
- **Generic/template parameters only guarantee their declared minimal interface**: a stdlib method like `find_first_of` exists for `std::string`, not for every `StringType` satisfying the template constraint — the custom type may implement only the operations the old code used. Same for duck-typed params (Python protocols, JS objects), C# generic constraints, and Rust trait bounds: replacing manual loops with library calls on the *concrete* type can fail to compile or silently dispatch differently. Compile the custom-type fixture before claiming equivalence; otherwise keep the manual loop.
- Conditional compilation means one build ≠ all code: C/C++/C# `#if HAVE_*`, Rust `cfg`/features, Kotlin `expect/actual` targets compile only some branches per platform. A simplification that compiles on your host may break another target's branch or its custom types.

## Errors & contracts
- Adding context (`%w`, `raise X from e`, new message) changes error text that callers/tests/log parsers may match.
- panic/exception ↔ `Result`/error return/`null` is an API change. So is a new exception type.
- Fixed buffers → growable containers drop size limits and their exceptions.
- `Promise.all`/`gather` changes call order and side-effect order (see Async).

## SQL
- Moving a predicate on the outer table from `ON` to `WHERE` turns a `LEFT JOIN` into an inner join.
- `COUNT(*)` vs `COUNT(col)` after an outer join (1 vs 0 for no match).
- `COALESCE(sum, 0)` changes NULL to 0; correlated subquery → join + `GROUP BY` can duplicate rows if the join fans out.
- `NOT IN (subquery)` with any NULL returns no rows; `NOT EXISTS` doesn't.
- NULL sort position is engine-specific; `ORDER BY 3` → alias must point to the same column.

## Shell
- `for f in $(ls dir)` → `for f in dir/*`: with no matches the glob stays literal unless `shopt -s nullglob`.
- Adding `set -e`: `((count++))` returns 1 when count is 0 and exits; failing commands that used to continue now abort. `set -u` breaks `$1` checks. Treat strict mode as a behavior change.
- Quoting fixes make names with spaces work: a desirable bug fix, but still a behavior change. Report it.

## Numbers
- Reordering floating-point operations (factor out, sum then discount vs discount then sum) changes rounding.
- Integer division vs float division; `Math.round` vs banker's rounding; `/` in Python 2 vs 3.
- JS `reduce` without an initial value throws on empty arrays; with `0` it returns `0`.
