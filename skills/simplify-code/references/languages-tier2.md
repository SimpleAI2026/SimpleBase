# Tier 2 Languages

Same structure as tier 1: **Style** · **Redundant → simpler** · **⚠ Keep equivalent**. Read only the section you need.

## Contents
Ruby · Dart · R · Scala · Lua · Objective-C · Assembly · Perl · MATLAB · Julia · Zig · Elixir · Haskell · Groovy · VB/VBA · Fortran · GDScript · Nix

## Ruby
- **Style:** [Ruby Style Guide](https://rubystyle.guide/), [clean-code-ruby](https://github.com/uohzxela/clean-code-ruby), RuboCop.
- **Redundant → simpler:** `each` + `<<` into new array → `map`/`select`/`filter_map`; nested `if` → `return … if`/`unless` guards; `x.nil? ? nil : x.y` → `x&.y`; explicit `return` on last line → drop; `self.attr` reads in methods → `attr`; hand-written readers → `attr_reader`.
- **⚠ Keep equivalent:** only `nil`/`false` are falsy; `&.` ≠ `try` (Rails `try` ignores missing methods); `filter_map` drops `false` too; `self.x = …` is required for setters; frozen string literals make `<<` raise. `each`/`map` **return the receiver**, while `detect`/`find` return the match or `nil` — converting control flow between them changes the method's final value, so add an explicit `nil` when a `detect` result is dropped.

## Dart
- **Style:** [Effective Dart](https://dart.dev/effective-dart), `dart format`, `package:lints`/`flutter_lints`.
- **Redundant → simpler:** one-statement bodies → `=>`; `x != null ? x : d` → `x ?? d`; `if (x == null) x = d` → `x ??= d`; loop+add → collection `for`/`if` in literals; constructor field assignments → `this.field` params; giant `build()` → private widgets.
- **⚠ Keep equivalent:** adding `const` changes identity (canonicalized instances); extracting a helper *method* vs a widget changes rebuild/element identity; `!` moves the null error; `==` override needs `hashCode`.

## R
- **Style:** [tidyverse style](https://style.tidyverse.org/), styler/lintr.
- **Redundant → simpler:** element loops → vectorized ops/`vapply`; `if (cond) TRUE else FALSE` → `cond`; repeated pipelines → named function; `sapply` → `vapply` with explicit type for stability.
- **⚠ Keep equivalent:** recycling with mismatched lengths; `sapply` may return list or matrix; `ifelse` drops attributes and evaluates both branches; `NA` propagation (`if (NA)` errors); `$` partial matching on lists/data frames; factor levels.

## Scala
- **Style:** [Scala Style Guide](https://docs.scala-lang.org/style/), scalafmt, Scalafix.
- **Redundant → simpler:** `opt match { case Some(x) => Some(f(x)); case None => None }` → `opt.map(f)`; nested `flatMap` → `for` comprehension; `if (c) true else false` → `c`; `return` → last expression; mutable `var` accumulation → `foldLeft`/`map`.
- **⚠ Keep equivalent:** `return` inside lambdas is non-local; views/`LazyList` defer evaluation; `getOrElse` is by-name (lazy) — fine; `case class` gives structural equality (API); implicit/given resolution can change when moving code.

## Lua
- **Style:** project guide, Luacheck, StyLua.
- **Redundant → simpler:** implicit globals → `local`; duplicated branch tails → shared code; repeated table access → local alias (only reads); `if x == nil then x = d end` → `x = x or d` when `false` is not a valid value.
- **⚠ Keep equivalent:** `a and b or c` breaks when `b` is `false`/`nil`; `x or d` replaces `false`; `#t` with holes is undefined; `ipairs` stops at the first nil, `pairs` order is unspecified; metatables (`__index`) make reads side-effectful.

## Objective-C
- **Style:** [Google ObjC Style](https://google.github.io/styleguide/objcguide.html), Apple conventions, clang-format.
- **Redundant → simpler:** `if (obj != nil) [obj doThing];` → `[obj doThing];` (nil messaging is a no-op) for void/object returns; explicit ivars + accessors → `@property`; `[NSArray arrayWithObjects:…, nil]` → literals `@[…]`; repeated setup → private method.
- **⚠ Keep equivalent:** nil messaging returns `0`/`nil` for scalars, but struct returns may be garbage on older runtimes; literals throw on `nil` elements while `arrayWithObjects:` stops at the first nil; KVO notifications fire for property setters, not direct ivar writes; block capture of `self` cycles.

## Assembly
- **Style:** project/ABI conventions; consistent syntax (AT&T vs Intel); comment intent, not the mnemonic.
- **Redundant → simpler:** dead stores and redundant `mov`s → remove **after** liveness analysis; identical branch tails → shared label; magic offsets → named `.equ`/`EQU` constants; numeric labels → descriptive labels.
- **⚠ Keep equivalent:** flags set as side effects of instructions; callee-saved registers and stack alignment; memory-mapped I/O and volatile reads; instruction size/alignment for patching or jump tables; delay slots. Never reorder without a dependency analysis.

## Perl
- **Style:** perltidy, Perl::Critic, `use strict; use warnings;`.
- **Redundant → simpler:** loop+push → `map`/`grep`; temp copies for a substitution → `s///r`; two-arg `open` → three-arg lexical `open my $fh, '<', $path`; repeated regex setup → named sub; `if (!…)` → `unless` for simple conditions only.
- **⚠ Keep equivalent:** scalar vs list context (`my $n = @list` vs `my ($first) = @list`); `$_` aliasing — `map`/`for` modify the original; `s///` mutates while `s///r` returns; adding `use strict` can break previously running code.

## MATLAB
- **Style:** [MATLAB coding guidelines](https://www.mathworks.com/help/matlab/matlab_prog/coding-and-readability.html), Code Analyzer.
- **Redundant → simpler:** element loops → vectorized/broadcast expressions; growing arrays in loops → preallocate or one expression; repeated blocks → local functions; `if c, r = true; else, r = false; end` → `r = c;`; `eval` building names → arrays/structs.
- **⚠ Keep equivalent:** `*` vs `.*`, `'` (conjugate transpose) vs `.'`; implicit expansion silently broadcasts mismatched shapes; row vs column orientation of results; scripts share the caller's workspace, functions don't; floating summation order.

## Julia
- **Style:** [Blue Style](https://github.com/invenia/BlueStyle) / SciML style, JuliaFormatter.
- **Redundant → simpler:** index loops → `for x in xs` / `eachindex`; push loops → comprehensions or `map`; `if/elseif` on types → multiple dispatch; repeated elementwise calls → `@.` broadcasting; `return` at end → last expression.
- **⚠ Keep equivalent:** broadcasting shapes; views (`@view`) alias, slices copy; `!` functions mutate; `==` vs `===` and `missing` propagation; type stability changes performance, not results — don't trade readability for it unasked.

## Zig
- **Style:** `zig fmt`, std library conventions.
- **Redundant → simpler:** `x catch |e| return e` → `try x`; per-path cleanup → `defer`/`errdefer` right after acquisition; `if (opt) |v| v else d` → `opt orelse d`; repeated allocations → caller-provided allocator patterns already in project.
- **⚠ Keep equivalent:** `defer` order is reverse; `errdefer` runs only on error return; `try` widens the inferred error set; `orelse unreachable` is UB in ReleaseFast; explicit allocator ownership.

## Elixir
- **Style:** [Elixir Style Guide](https://github.com/christopheradams/elixir_style_guide), `mix format`, Credo.
- **Redundant → simpler:** nested `case` on `{:ok, _}` → `with`; temporary variables chained → `|>` pipeline; `if` on shape → pattern-matched function heads; `Enum.map |> Enum.filter` re-traversals → one `for` comprehension when clearer.
- **⚠ Keep equivalent:** `with` returns the first non-matching value unless `else` is given; clause order matters in multi-head functions; `Enum` is eager, `Stream` lazy; `String.to_atom` on user input leaks atoms.

## Haskell
- **Style:** Ormolu/Fourmolu, HLint (its hints are usually the minimal idiom).
- **Redundant → simpler:** `case m of Nothing -> Nothing; Just x -> Just (f x)` → `fmap f m`; nested `case` on `Either` → `do`/`traverse`; `\x -> f x` → `f`; `if c then True else False` → `c`; explicit recursion that is a fold → `foldr`/`foldl'`.
- **⚠ Keep equivalent:** strictness/laziness (`foldl` vs `foldl'`, adding bang patterns); bottom and partial functions (`head`); point-free can hide argument order; sharing (`let` bound values computed once).

## Groovy
- **Style:** CodeNarc, surrounding Java/Gradle conventions.
- **Redundant → simpler:** loop+add → `collect`/`findAll`; `x != null ? x.y : null` → `x?.y`; `x ? x : d` → `x ?: d`; explicit getters/setters → properties; semicolons/`return` noise → drop per project style.
- **⚠ Keep equivalent:** Groovy truth (`0`, `''`, empty collections are false) — `?:` replaces them; `==` calls `equals`/`compareTo`; GString vs String as map keys; Jenkins CPS can't serialize some closures.

## VB / VBA
- **Style:** `Option Explicit`, Rubberduck inspections.
- **Redundant → simpler:** repeated object qualification → `With … End With`; `If c Then x = True Else x = False` → `x = c`; duplicated subs → one parameterized `Sub`/`Function`; `.Select`/`Selection` chains → direct range references.
- **⚠ Keep equivalent:** default properties (`Range` → `.Value`); parameters are `ByRef` by default; `Variant` coercion; `On Error Resume Next` scope; removing `.Select` changes the visible selection the user may rely on.

## Fortran
- **Style:** [fortran-lang best practices](https://fortran-lang.org/learn/best_practices/), `implicit none`, fprettify.
- **Redundant → simpler:** element `do` loops → whole-array expressions; repeated code → internal/module procedures; `common` blocks → module variables (API-level — propose); hard-coded kinds → named kind parameters.
- **⚠ Keep equivalent:** array expressions evaluate the full RHS before assignment (overlapping arrays differ from a loop); column-major order; assumed-shape needs explicit interfaces; floating evaluation order; `intent(out)` deallocates allocatables.

## GDScript (Godot)
- **Style:** [GDScript style guide](https://docs.godotengine.org/en/stable/tutorials/scripting/gdscript/gdscript_styleguide.html), built-in formatter, gdtoolkit.
- **Redundant → simpler:** repeated `get_node("…")` → `@onready var` (Godot 4) or `%UniqueName`; nested ifs in `_process` → guards; duplicated signal handlers → one handler with bound args; untyped vars → typed where the project uses static typing.
- **⚠ Keep equivalent:** `@onready` resolves once at ready time — nodes added/replaced later aren't seen; signal connection order; `await` across freed objects; `is_instance_valid` checks.

## Nix
- **Style:** [nixpkgs conventions](https://github.com/NixOS/nixpkgs/blob/master/CONTRIBUTING.md), `nixfmt`, statix/deadnix.
- **Redundant → simpler:** repeated expressions → `let … in`; `x = x;` → `inherit x;`; `a.b.c = …; a.b.d = …;` nesting → one attrset; dead bindings (deadnix) → remove; nested `with` → explicit `pkgs.foo`.
- **⚠ Keep equivalent:** `with` scope has lower priority than `let`; `rec` attrsets and override order (`//` is right-biased, shallow); laziness hides errors until evaluated; any change to derivation inputs changes the hash.
