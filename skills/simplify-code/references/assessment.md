# Simplification assessments and constrained verification

## Mode and scope

A read-only assessment proposes behavior-preserving simplifications; it is not a general bug hunt or security audit. If the user says "analysis only", don't apply demonstration edits, add characterization tests, create a branch, commit, or push. If test/build commands would write into the checkout, inspect their configuration instead or propose a separately authorized disposable copy. Requested report files are the only writes in this mode.

For large repositories:
1. Start with named files/modules; otherwise use current-session changes or the working diff.
2. For an explicit broad assessment with no files named, choose a bounded sample: representative utility modules, high-churn files when local history is available, cohesive areas with observable duplication/nesting, and their immediate callers/tests. Don't search the entire tree for every smell.
3. Exclude vendor/generated/build artifacts by default. Inspect a public utility's downstream contracts before suggesting simplification.
4. State the revision if available, sampled files/line ranges and why chosen, exclusions, and limits. Absence of findings in a sample isn't evidence that the whole repository is clean.
5. Expand only when a shared contract or call site requires it. Avoid a quota of changes: mature code may deserve no edits.

## Priorities, risk, and evidence

Priority is expected maintainability benefit, not bug severity, urgency, or automatic permission to apply. Keep risk and confidence separate.

| Priority | Meaning | Typical candidate |
|---|---|---|
| P1 | Large recurring maintenance burden; clear payoff from reducing substantial duplicated knowledge or untangling responsibilities | Repeated full implementations of the same business operation; cohesive decomposition of a god method |
| P2 | Meaningful local simplification or duplicated rule with bounded benefit | Guard clauses, recurring validation logic, confirmed unused private code |
| P3 | Optional small readability improvement; omission is acceptable | Local names, redundant temporary, equivalent concise idiom |

Risk: **low** = local and clearly equivalent; **medium** = shared private logic, resources, or evaluation count; **high** = public contracts, generic/custom types, concurrency, serialization, or multiple platform branches. Any discovered behavior/API difference is **needs approval**, not "safe P1".

Evidence/confidence: **verified in stated scope** (actual commands and fixtures), **reasoned** (contract inspected, not executed), or **unresolved** (unknown compatibility/callers). Tests strengthen confidence; they don't prove universal equivalence.

### Assessment report

```
Mode: read-only simplification assessment; source unchanged.
Scope: revision, selected files/modules, selection rationale, exclusions.
Findings:
| ID | Priority | file:line / symbol | Redundancy → proposal | Benefit | Risk | Evidence | Approval needed |
Not changed: already-simple code, lint/compatibility restrictions, rejected rewrites.
Bugs found (not fixed): suspected/confirmed; location; concrete input or reproduction; consequence.
Verification: command + exit/result + baseline; not run + reason; untested platforms/types.
```

Use concise rows, group by responsibility, and avoid requiring every project to have P1 findings. Keep incidental defects outside simplification counts; don't patch them silently. A plausible bug without a reproducer is "suspected", not "confirmed". Diagnostics, error text, and stderr count are observable behavior too.

## Project lint versus skill preferences

Behavior and APIs first, then supported platform/runtime and project lint/configuration, then skill style preferences. A "cleaner" rewrite that introduces a warning is not finished. Compare warning sets against baseline; try a meaningful helper only if it reduces overall complexity and passes lint. If no worthwhile compatible rewrite exists, retain the original, even a nested ternary. Don't disable rules, add suppressions, install formatters, or upgrade the runtime to force a simplification.

## Verification when tools or time are constrained

- Check required versions and platform targets before choosing syntax. A newer local compiler doesn't make a Java 8/Android, old Go, or minimum-supported-Rust target accept newer APIs.
- Use already configured CI scripts, focused tests, type checks, and representative compile units before considering a heavy full build. Include custom types and target-specific branches touched by the change. Do not install tools or dependencies unless requested.
- If a tool is missing, too old, or a build is impractical, record **not run**, the actual reason, and the exact command/target needed later. Fall back to caller/contract inspection and small executable checks using existing tools; label them as partial, not full project verification.
- Don't bypass version gates (`--ignore-rust-version`, altered engines, disabled lint) to claim compatibility. If a user explicitly authorizes exploratory bypass checks, label them as unsupported-environment experiments.
- Record pre-edit failures and warning sets separately from new ones. An unchanged set of failures means "no new failures in this run", not "tests passed"; an unsupported runtime leaves validation incomplete.
- A build on one platform only validates compiled branches. List untouched/uncompiled C/C++/C# preprocessor paths, Rust cfg/features, Kotlin expect/actual targets, and Swift deployment targets. Don't claim all-platform compatibility from a host build.
- In edit mode, fix a newly introduced failure or undo just that candidate. Missing-tool uncertainty should favor leaving high-risk generic/public/platform code unchanged and proposing it instead.

## Lessons from representative libraries

- **Generic string abstraction:** replacing a character loop with `find_first_of` works for std::string but may fail for a custom StringType with a smaller interface. Compile the custom-type fixture or keep the generic implementation.
- **Lint conflict:** replacing a nested ternary with nested ifs may trigger max-depth. Project lint wins; a new warning requires a different rewrite or retaining the original.
- **Command batching:** five `type` calls → one `type a b c d e` may keep stdout but change stderr count/text and status. Compare failures as well as success output.
- **Compatibility floor:** String.repeat is unavailable on Java 8/Android targets. A mature compatible implementation may already be the simplest viable version.
