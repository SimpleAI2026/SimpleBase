# Sources

Rules in this skill are derived from foundational books, Fowler's catalog/bliki, Ousterhout's design philosophy, language clean-code adaptations, official style guides, and recent practitioner discussion (including X). Language-tier choices are informed by TIOBE, Stack Overflow surveys, and GitHub Octoverse popularity — not as a quality ranking.

## Books
- Robert C. Martin (Uncle Bob), *Clean Code: A Handbook of Agile Software Craftsmanship* (2008)
- Martin Fowler (with Kent Beck et al.), *Refactoring: Improving the Design of Existing Code*, 2nd ed. (2018) — catalog: https://refactoring.com/catalog/
- Steve McConnell, *Code Complete*, 2nd ed. (2004)
- John Ousterhout, *A Philosophy of Software Design* (APoSD) — and the comparison notes: https://github.com/johnousterhout/aposd-vs-clean-code
- Andrew Hunt & David Thomas, *The Pragmatic Programmer* (1999; 20th anniversary ed. 2019)
- Jeff Bay, *Object Calisthenics* (in *The ThoughtWorks Anthology*) — nine optional strict exercises

## Fowler (bliki, catalog, articles)
- CodeSmell: https://martinfowler.com/bliki/CodeSmell.html
- TechnicalDebt: https://martinfowler.com/bliki/TechnicalDebt.html
- Refactoring catalog: https://refactoring.com/catalog/
- *Exploring Gen-AI: The Economic Benefit of Refactoring*: https://martinfowler.com/articles/exploring-gen-ai/refactoring-economic-benefit.html
  — modularity/refactoring can cut agent token cost; clean structure has economic value beyond humans-only maintenance

## Clean-code language repos
- JavaScript: https://github.com/ryanmcdermott/clean-code-javascript
- TypeScript: https://github.com/labs42io/clean-code-typescript
- Python: https://github.com/zedr/clean-code-python
- PHP: https://github.com/piotrplenik/clean-code-php (canonical; earlier mirrors such as jupeter/clean-code-php may redirect or lag)
- Ruby: https://github.com/uohzxela/clean-code-ruby
- .NET/C#: https://github.com/thangchung/clean-code-dotnet
- Java: https://github.com/leonardolemie/clean-code-java
- Go: https://github.com/Pungyeon/clean-go-article
- Related list: https://github.com/kkisiele/awesome-clean-code-resources

## Official / de-facto style guides
- Python PEP 8: https://peps.python.org/pep-0008/ · PEP 20: https://peps.python.org/pep-0020/
- Effective Go: https://go.dev/doc/effective_go · Code Review Comments: https://go.dev/wiki/CodeReviewComments
- C++ Core Guidelines: https://isocpp.github.io/CppCoreGuidelines/
- Rust API Guidelines: https://rust-lang.github.io/api-guidelines/
- Swift API Design Guidelines: https://www.swift.org/documentation/api-design-guidelines/
- Kotlin coding conventions: https://kotlinlang.org/docs/coding-conventions.html
- Google Style Guides (JS/TS/C++/Swift/Java/Shell/HTML-CSS/ObjC, etc.): https://google.github.io/styleguide/
- .NET coding conventions: https://learn.microsoft.com/dotnet/csharp/fundamentals/coding-style/coding-conventions
- PHP PSR-12: https://www.php-fig.org/psr/psr-12/ · PSR-1: https://www.php-fig.org/psr/psr-1/
- Ruby Style Guide: https://rubystyle.guide/
- Effective Dart: https://dart.dev/effective-dart
- Elixir style: https://github.com/christopheradams/elixir_style_guide
- GDScript: https://docs.godotengine.org/en/stable/tutorials/scripting/gdscript/gdscript_styleguide.html

## X / practitioner highlights (Oct 2026 and ongoing)
- **@unclebobmartin** — clean code gets code out of the way of thinking; agents need clean code too; AI reduces syntax cost but not the need for design thinking. Profile: https://x.com/unclebobmartin
- **@martinfowler** — Sensible Defaults; refactoring's economic benefit including lower agent token cost. Profile: https://x.com/martinfowler · article above on economic benefit of refactoring
- Balance with Ousterhout's critique (APoSD / aposd-vs-clean-code): treat Clean Code heuristics as **guidelines, not dogma** — prefer deep modules over tiny-function theater

## Ranking context (which languages to document)
- TIOBE Index: https://www.tiobe.com/tiobe-index/
- Stack Overflow Developer Survey: https://survey.stackoverflow.co/
- GitHub Octoverse: https://github.blog/octoverse/

These inform tier placement in `languages.md` (popularity and practical encounter rate for agents), not absolute importance.
