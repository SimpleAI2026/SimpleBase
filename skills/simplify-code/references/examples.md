# Before / After Examples

Three groups. **A** = safe to apply in private or public code (contract preserved). **B** = improves design but changes behavior or API — *propose only*. **C** = tempting rewrites to reject, plus already-minimal code. **D** = report samples. Every example names the contract it preserves; if your case differs in that detail, the rewrite may not be safe.

## Contents
- A. Safe simplifications: A1 guard clauses (JS) · A2 boolean return (C#) · A3 comprehension (Python) · A4 enumerate/zip (Python) · A5 counting (Python) · A6 optional chaining (TS) · A7 happy path (Go) · A8 stdlib search (Go) · A9 `?` and `map` (Rust) · A10 pass-through helpers (Java) · A11 merged switch (Java) · A12 null operators + `using` (C#) · A13 RAII (C++) · A14 `any_of` (C++) · A15 single cleanup (C) · A16 null safety + `when` (Kotlin) · A17 `guard let` (Swift) · A18 `??` + promotion (PHP) · A19 CTE + COALESCE (SQL) · A20 shell plumbing (Bash) · A21 splatting (PowerShell) · A22 Enumerable + guards (Ruby) · A23 `with` (Elixir) · A24 `fmap` (Haskell) · A25 locals (Lua) · A26 vectorization (R) · A27 wrappers + shorthand (HTML/CSS) · A28 dead code + noise comments · A29 shallow helpers → one function · A30 duplicated knowledge · A31 lookup table · A32 magic numbers · A33 `with` (Python) · A34 extract predicate (Java)
- B. Propose only: B1 flag argument · B2 parameter object · B3 mutation → copy · B4 error wrapping (Go) · B5 `unwrap` → `Result` (Rust) · B6 callback → async · B7 newtype · B8 swallowed error · B9 SQL precedence · B10 shell quoting/globs · B11 `switch` → `match` (PHP)
- C. Reject / leave alone: C1 `||` → `??` · C2 filter-then-act reorder · C3 list → generator · C4 nil vs empty slice · C5 compressed one-liner · C6 wrong abstraction · C7 `toList()` mutability · C8 `NOT IN` · C9 already minimal · C10 `toMap` (Java) · C11 `for..in` + `sort()` (TS) · C12 "strict everywhere" (PHP) · C13 `await` loop → `Promise.all` (TS) · C14 removing `any` without re-running `tsc`
- D. Report samples

---

## A. Safe simplifications

### A1. Guard clauses (JS)
```js
// Before
export function label(value) {
  if (value == null) {
    return 'missing';
  } else {
    if (value === 0) {
      return 'zero';
    } else {
      return value || 'empty';
    }
  }
}
// After
export function label(value) {
  if (value == null) return 'missing';
  if (value === 0) return 'zero';
  return value || 'empty';
}
```
Preserves: `== null` (catches `undefined`), `||` fallback for `false`/`''`/`NaN`. Don't "modernize" to `??` or `===` here (see C1).

### A2. Boolean return (C#)
```csharp
// Before
if (order.Total > 100) { return true; } else { return false; }
// After
return order.Total > FreeShippingThreshold; // const decimal FreeShippingThreshold = 100;
```
Preserves: the comparison already yields `bool`. In JS/Python, `return cond` is only identical when `cond` is a real boolean.

### A3. Append loop → comprehension (Python)
```python
# Before
def active_names(users):
    result = []
    for user in users:
        if user["active"]:
            result.append(user["name"])
    return result
# After
def active_names(users):
    return [user["name"] for user in users if user["active"]]
```
Preserves: eager `list`, order, duplicates, `KeyError` on missing keys. Don't switch to a generator or `.get()`.

### A4. Index loops → `enumerate` / `zip` (Python)
```python
# Before
for i in range(len(names)):
    print(i, names[i], scores[i])
# After
for i, (name, score) in enumerate(zip(names, scores)):
    print(i, name, score)
```
⚠ `zip` stops at the shorter list; the original raised `IndexError` if `scores` was shorter. If lengths may differ, use `zip(names, scores, strict=True)` (3.10+, raises `ValueError` — different type) or keep the index loop.

### A5. Counting boilerplate (Python)
```python
# Before
counts = {}
for word in words:
    if word in counts:
        counts[word] += 1
    else:
        counts[word] = 1
return counts
# After
return dict(Counter(words))
```
Preserves: plain `dict`, first-seen key order, `KeyError` for missing words. Returning the `Counter` itself would change missing-key behavior (returns 0).

### A6. Optional chaining (TS)
```ts
// Before
const city = user && user.address && user.address.city ? user.address.city : 'unknown';
// After
const city = user?.address?.city || 'unknown';
```
Preserves: the original fell back on *any* falsy city (`''` too), so keep `||`, not `??`. Falsy `user`/`address` values still yield `'unknown'`. Only difference: `user.address.city` is read once instead of twice — matters only if it's a side-effecting getter.

### A7. Left-aligned happy path (Go)
```go
// Before
data, err := os.ReadFile(path)
if err == nil {
    cfg, err := parse(data)
    if err == nil {
        return cfg, nil
    } else {
        return nil, err
    }
} else {
    return nil, err
}
// After
data, err := os.ReadFile(path)
if err != nil {
    return nil, err
}
return parse(data)
```
Preserves: exact errors and values, provided `parse` returns `(*Config, error)` with `nil` config on error. Adding `fmt.Errorf` context is B4.

### A8. Hand-written search → stdlib (Go 1.21+)
```go
// Before
found := false
for _, r := range roles {
    if r == "admin" {
        found = true
        break
    }
}
// After
found := slices.Contains(roles, "admin")
```
Only if `go.mod` declares ≥1.21; otherwise keep the loop.

### A9. `?` and `map` (Rust)
```rust
// Before
let text = match fs::read_to_string(path) {
    Ok(t) => t,
    Err(e) => return Err(e),
};
let port = match cfg.port {
    Some(p) => Some(p + 1),
    None => None,
};
// After
let text = fs::read_to_string(path)?;
let port = cfg.port.map(|p| p + 1);
```
Preserves: the function already returns `Result<_, io::Error>`, so `?`'s `From` conversion is the identity.

### A10. Inline pass-through helpers (Java)
```java
// Before
private String normalizedName(User u) { return normalize(u.getName()); }
private String normalize(String s) { return trimAndLower(s); }
private String trimAndLower(String s) { return s.trim().toLowerCase(Locale.ROOT); }
// After
private String normalizedName(User u) { return u.getName().trim().toLowerCase(Locale.ROOT); }
```
Only private, single-use hops with no added meaning. Keep a helper if its name documents a domain rule or it's reused.

### A11. Merge identical branches (Java)
```java
// Before
switch (status) {
    case PENDING: return "open";
    case IN_REVIEW: return "open";
    case DONE: return "closed";
    default: return "unknown";
}
// After (Java 14+)
return switch (status) {
    case PENDING, IN_REVIEW -> "open";
    case DONE -> "closed";
    default -> "unknown";
};
```
Preserves: keeps `default`; a `null` `status` throws NPE in both forms.

### A12. Null operators and `using` declaration (C#)
```csharp
// Before
string name;
if (customer != null && customer.Name != null) { name = customer.Name; } else { name = "guest"; }
using (var reader = new StreamReader(path))
{
    return Parse(reader.ReadToEnd(), name);
}
// After
var name = customer?.Name ?? "guest";
using var reader = new StreamReader(path);
return Parse(reader.ReadToEnd(), name);
```
Preserves: disposal still happens after `Parse` returns because `return` is the last statement. If more code followed the block, `using var` would extend the lifetime — keep the block.

### A13. RAII (C++)
```cpp
// Before
Foo* f = new Foo();
if (!f->init()) { delete f; return false; }
use(f);
delete f;
return true;
// After
auto f = std::make_unique<Foo>();
if (!f->init()) return false;
use(f.get());
return true;
```
Preserves: `use` takes a non-owning `Foo*` and doesn't store it; also fixes the leak if `use` throws (report as a bonus fix).

### A14. Hand-written algorithm → `<algorithm>` (C++)
```cpp
// Before
bool hasNegative = false;
for (size_t i = 0; i < v.size(); ++i) {
    if (v[i] < 0) { hasNegative = true; break; }
}
// After
const bool hasNegative = std::any_of(v.begin(), v.end(), [](int x) { return x < 0; });
```
Preserves: short-circuits at the first match like the `break`.

### A15. Single cleanup path (C)
```c
/* Before */
int load(const char *path) {
    FILE *f = fopen(path, "r");
    if (!f) return -1;
    char *buf = malloc(SIZE);
    if (!buf) { fclose(f); return -1; }
    if (fread(buf, 1, SIZE, f) == 0) { free(buf); fclose(f); return -1; }
    process(buf);
    free(buf);
    fclose(f);
    return 0;
}
/* After */
int load(const char *path) {
    int rc = -1;
    char *buf = NULL;
    FILE *f = fopen(path, "r");
    if (!f) return -1;
    buf = malloc(SIZE);
    if (!buf) goto cleanup;
    if (fread(buf, 1, SIZE, f) == 0) goto cleanup;
    process(buf);
    rc = 0;
cleanup:
    free(buf);
    fclose(f);
    return rc;
}
```
Preserves: same return codes and release order (`free` then `fclose`); `free(NULL)` is a no-op.

### A16. Null safety and merged `when` (Kotlin)
```kotlin
// Before
val email = if (user != null && user.email != null) user.email else "none"
val label = when (code) {
    1 -> "low"
    2 -> "low"
    3 -> "high"
    else -> "unknown"
}
// After
val email = user?.email ?: "none"
val label = when (code) {
    1, 2 -> "low"
    3 -> "high"
    else -> "unknown"
}
```

### A17. Pyramid → `guard let` (Swift)
```swift
// Before
func avatarURL(for user: User?) -> URL? {
    if let user = user {
        if let path = user.avatarPath {
            return URL(string: base + path)
        }
    }
    return nil
}
// After
func avatarURL(for user: User?) -> URL? {
    guard let path = user?.avatarPath else { return nil }
    return URL(string: base + path)
}
```

### A18. `??` and constructor promotion (PHP 8+)
```php
// Before
class Mailer {
    private string $host;
    private int $port;
    public function __construct(string $host, int $port) {
        $this->host = $host;
        $this->port = $port;
    }
}
$timeout = isset($options['timeout']) ? $options['timeout'] : 30;
// After
class Mailer {
    public function __construct(private string $host, private int $port) {}
}
$timeout = $options['timeout'] ?? 30;
```
Preserves: `isset` and `??` are identical (both treat `null` as missing). Promotion keeps visibility and types; check the project's PHP version.

### A19. Repeated subquery → CTE, CASE → COALESCE (SQL)
```sql
-- Before
SELECT u.id,
       CASE WHEN u.nickname IS NULL THEN u.name ELSE u.nickname END AS display_name,
       (SELECT SUM(amount) FROM payments p WHERE p.user_id = u.id AND p.status = 'paid') AS paid
FROM users u
WHERE (SELECT SUM(amount) FROM payments p WHERE p.user_id = u.id AND p.status = 'paid') > 100;
-- After
WITH paid_totals AS (
    SELECT user_id, SUM(amount) AS paid
    FROM payments
    WHERE status = 'paid'
    GROUP BY user_id
)
SELECT u.id,
       COALESCE(u.nickname, u.name) AS display_name,
       t.paid
FROM users AS u
JOIN paid_totals AS t ON t.user_id = u.id
WHERE t.paid > 100;
```
Preserves: users without payments had `SUM = NULL`, and `NULL > 100` filtered them out, so the inner join drops the same rows. If the outer `WHERE` were removed, you'd need `LEFT JOIN` to keep them.

### A20. Shell plumbing (Bash)
```bash
# Before
count=$(cat "$log" | grep -c ERROR)
deploy
if [ $? -ne 0 ]; then
  echo "deploy failed" >&2
  exit 1
fi
# After
count=$(grep -c ERROR "$log")
if ! deploy; then
  echo "deploy failed" >&2
  exit 1
fi
```
Preserves: same count and exit path, *assuming the script doesn't use `set -e`* — with `set -e` the original aborts at `deploy` without the message, while `if ! deploy` prints it (a behavior change to report).

### A21. Splatting (PowerShell)
```powershell
# Before
Invoke-RestMethod -Uri $url -Method Post -Headers $headers -Body $body -ContentType 'application/json' -TimeoutSec 30
Invoke-RestMethod -Uri $url2 -Method Post -Headers $headers -Body $body2 -ContentType 'application/json' -TimeoutSec 30
# After
$common = @{ Method = 'Post'; Headers = $headers; ContentType = 'application/json'; TimeoutSec = 30 }
Invoke-RestMethod -Uri $url -Body $body @common
Invoke-RestMethod -Uri $url2 -Body $body2 @common
```

### A22. Enumerable and guards (Ruby)
```ruby
# Before
def emails(users)
  if users
    result = []
    users.each do |u|
      if u.active
        result << u.email
      end
    end
    return result
  else
    return []
  end
end
# After
def emails(users)
  return [] unless users

  users.select(&:active).map(&:email)
end
```
Preserves: Ruby truthiness — only `nil`/`false` hit the guard, as before.

### A23. Nested `case` → `with` (Elixir)
```elixir
# Before
def register(params) do
  case validate(params) do
    {:ok, attrs} ->
      case Repo.insert(User.changeset(attrs)) do
        {:ok, user} -> {:ok, user}
        {:error, cs} -> {:error, cs}
      end
    {:error, reason} -> {:error, reason}
  end
end
# After
def register(params) do
  with {:ok, attrs} <- validate(params) do
    Repo.insert(User.changeset(attrs))
  end
end
```
Preserves: a non-matching value is returned unchanged by `with`, and `Repo.insert` already returns `{:ok, _} | {:error, _}`.

### A24. `case` plumbing → `fmap` (Haskell)
```haskell
-- Before
userName :: Maybe User -> Maybe String
userName mu = case mu of
  Nothing -> Nothing
  Just u  -> Just (name u)
-- After
userName :: Maybe User -> Maybe String
userName = fmap name
```

### A25. Implicit globals and duplicate tails (Lua)
```lua
-- Before
function price(item, vip)
  total = item.base * item.qty
  if vip then
    total = total * 0.9
    return math.floor(total)
  else
    return math.floor(total)
  end
end
-- After
local VIP_DISCOUNT = 0.9

function price(item, vip)
  local total = item.base * item.qty
  if vip then total = total * VIP_DISCOUNT end
  return math.floor(total)
end
```
⚠ Making `total` local is safe only if nothing else reads the global `total` — search first.

### A26. Element loop → vectorized (R)
```r
# Before
out <- numeric(length(x))
for (i in seq_along(x)) {
  out[i] <- x[i] * rate + fee
}
# After
out <- x * rate + fee
```
Preserves: `rate` and `fee` are scalars. If either is a vector, the loop errors (`replacement has length > 1`) while the vectorized form silently recycles — check before rewriting.

### A27. Wrapper div and shorthand (HTML/CSS)
```html
<!-- Before -->
<div class="card"><div><div class="card-title">Plan</div></div></div>
<!-- After -->
<div class="card"><h3 class="card-title">Plan</h3></div>
```
```css
/* Before */
.card { margin-top: 8px; margin-right: 0; margin-bottom: 8px; margin-left: 0; }
/* After */
.card { margin: 8px 0; }
```
Preserves: the inner `div` had no class, style, or selectors (check `.card > div`, `.card div`, tests, JS). `h3` brings default margins/font — reset or keep a `div` if styling must be pixel-identical; the semantic change is an accessibility improvement to report.

### A28. Dead code and noise comments (Python)
```python
# Before
import json
# 2016-12-20: removed monads (RM)
# def old_hash(data): ...
def hash_value(data):
    # loop over data
    h = 0
    for ch in data:
        h = ((h << 5) - h) + ord(ch)
        h &= 0xFFFFFFFF  # convert to 32-bit int
    return h
# After
def hash_value(data):
    h = 0
    for ch in data:
        h = ((h << 5) - h) + ord(ch)
        h &= 0xFFFFFFFF  # keep within 32-bit range (matches legacy JS hash)
    return h
```
Verified first: `json` unused in this module and not re-exported. The *why* comment survives.

### A29. Shallow helpers → one deep function (Python)
```python
# Before
def _open(path): return open(path)
def _read(f): return f.read()
def _parse(text): return json.loads(text)
def load_config(path):
    f = _open(path)
    try:
        return _parse(_read(f))
    finally:
        f.close()
# After
def load_config(path):
    with open(path) as f:
        return json.load(f)
```
Preserves: same exceptions, file closed on all paths. The helpers added hops without hiding anything.

### A30. Duplicated knowledge (TS)
```ts
// Before
function shippingForCart(cart: Cart) { return cart.total >= 50 ? 0 : 5.99; }
function shippingBanner(cart: Cart) { return cart.total >= 50 ? 'Free shipping!' : `Add ${50 - cart.total} for free shipping`; }
// After
const FREE_SHIPPING_MIN = 50;
const qualifiesForFreeShipping = (cart: Cart) => cart.total >= FREE_SHIPPING_MIN;

function shippingForCart(cart: Cart) { return qualifiesForFreeShipping(cart) ? 0 : 5.99; }
function shippingBanner(cart: Cart) {
  return qualifiesForFreeShipping(cart) ? 'Free shipping!' : `Add ${FREE_SHIPPING_MIN - cart.total} for free shipping`;
}
```
The threshold is one business rule that must change together — that's real duplication.

### A31. Type switch → lookup table (Python)
```python
# Before
def area(shape):
    if shape.kind == "circle": return math.pi * shape.r ** 2
    elif shape.kind == "square": return shape.side ** 2
    elif shape.kind == "rect": return shape.w * shape.h
# After
AREA = {
    "circle": lambda s: math.pi * s.r ** 2,
    "square": lambda s: s.side ** 2,
    "rect": lambda s: s.w * s.h,
}

def area(shape):
    compute = AREA.get(shape.kind)
    return compute(shape) if compute else None
```
Preserves: unknown kinds still return `None`. `AREA[shape.kind]` would raise `KeyError` — a behavior change.

### A32. Magic numbers (JS)
```js
// Before
setTimeout(blastOff, 86400000);
if (retries > 3) giveUp();
// After
const MS_PER_DAY = 24 * 60 * 60 * 1000;
const MAX_RETRIES = 3;
setTimeout(blastOff, MS_PER_DAY);
if (retries > MAX_RETRIES) giveUp();
```

### A33. Manual close → `with` (Python)
```python
# Before
f = open(path)
data = f.read()
f.close()
# After
with open(path) as f:
    data = f.read()
```
Bonus fix: the file now closes even if `read` raises — mention it.

### A34. Extract predicate (Java)
```java
// Before
if (!(user.getAge() >= 18 && !user.isBanned() && "US".equals(user.getCountry()))) { deny(); }
// After
if (!isEligible(user)) { deny(); }

private static boolean isEligible(User user) {
    return user.getAge() >= ADULT_AGE && !user.isBanned() && "US".equals(user.getCountry());
}
```
Preserves: same short-circuit order (`getAge` → `isBanned` → `getCountry`).

---

## B. Propose only (behavior or API changes)

Show these as suggestions with the concrete difference; apply only with approval or when the code is private and all callers are updated in scope.

### B1. Flag argument → two functions (Python)
```python
# Before
def create_file(name, temp): ...
# Proposed
def create_file(name): Path(name).touch()
def create_temp_file(name): Path("./temp/" + name).touch()
```
Public signature change. Compatible alternative: keep `create_file(name, temp=False)` as a thin wrapper.

### B2. Long parameter list → options object (TS)
```ts
// Before
export function createMenu(title: string, body: string, buttonText: string, cancellable: boolean) {}
// Proposed
export interface MenuOptions { title: string; body: string; buttonText: string; cancellable: boolean }
export function createMenu(options: MenuOptions) {}
```
Every caller changes. Apply directly only to non-exported functions.

### B3. Mutation → returning a copy (JS)
```js
// Before
function addItem(cart, item) { cart.push({ item, date: Date.now() }); }
// Proposed
function addItem(cart, item) { return [...cart, { item, date: Date.now() }]; }
```
Callers relying on in-place mutation would silently lose items.

### B4. Error context (Go)
```go
// Before
return nil, err
// Proposed
return nil, fmt.Errorf("read config %s: %w", path, err)
```
`errors.Is/As` still work, but `err.Error()` text changes — tests, log alerts, or API responses may match it.

### B5. `unwrap` → `Result` (Rust)
```rust
// Before
pub fn load(path: &Path) -> Config { serde_json::from_str(&fs::read_to_string(path).unwrap()).unwrap() }
// Proposed
pub fn load(path: &Path) -> Result<Config, LoadError> { /* map_err + ? */ }
```
Panic → `Err` changes the signature and every caller.

### B6. Callback → async (JS)
```js
// Before
export function fetchUser(id, cb) { db.get(id, cb); }
// Proposed
export async function fetchUser(id) { return db.getAsync(id); }
```
Callers passing `cb` stop being called; timing becomes microtask-based.

### B7. Primitive → newtype (TS)
```ts
// Before
export function send(email: string) {}
// Proposed
type Email = string & { readonly __brand: 'Email' };
export function send(email: Email) {}
```
Compile-time API change; offer a validating constructor at the boundary.

### B8. Swallowed error (JS)
```js
// Before
try { sync(); } catch (e) { console.log(e); }
// Proposed
try { sync(); } catch (e) { logger.error('profile sync failed', { error: e }); throw e; }
```
Rethrowing changes control flow for callers that expected silence — it's a bug fix, not a cleanup.

### B9. SQL with ambiguous precedence
```sql
-- Before
SELECT * FROM orders o, customers c WHERE o.cid = c.id AND o.total > 100 OR c.vip = 1;
```
`AND` binds tighter: VIP customers are joined to *every* order (cross join). Rewriting to `JOIN … ON` plus parenthesised `WHERE` fixes it — but that changes results. Ask which was intended; the cleanup-only rewrite must keep `WHERE (o.cid = c.id AND o.total > 100) OR c.vip = 1`.

### B10. Unquoted shell loop (Bash)
```bash
# Before
for f in $(ls $dir); do rm $f; done
# Proposed
for f in "$dir"/*; do rm -- "$f"; done
```
Fixes names with spaces/globs, but with an empty dir the glob stays literal (`rm: "$dir/*": No such file`) unless `shopt -s nullglob`; hidden files are still skipped. Report as a behavior fix.

### B11. `switch` → `match` (PHP)
```php
// Before
switch ($code) { case 1: return 'one'; case 2: return 'two'; default: return 'other'; }
// Proposed
return match ($code) { 1 => 'one', 2 => 'two', default => 'other' };
```
`switch` compares loosely: `'1'` and `true` matched `case 1`; `match` uses `===` and returns `'other'`. Safe only if `$code` is guaranteed `int` (typed parameter + `strict_types`).

---

## C. Reject or leave alone

### C1. `||` → `??` (JS)
```js
const pageSize = options.pageSize || 20;
```
Rewriting to `?? 20` changes `pageSize: 0` from `20` to `0`. If the user asks for it, decline that part citing `{ pageSize: 0 }`.

### C2. Reordering filter and side effects (JS)
```js
// Original: lookup runs once per client, email only for active ones
clients.forEach(c => { const r = db.lookup(c); if (r.isActive()) email(c); });
```
`clients.filter(isActive).forEach(email)` is equivalent only if `db.lookup` is pure. If it logs, caches, or throws partway, calls interleave differently (all lookups run before any email). Keep the loop or confirm purity.

### C3. List → generator (Python)
```python
def ids(rows): return [r.id for r in rows]
```
`(r.id for r in rows)` breaks callers that call `len()`, index, or iterate twice. Leave as a list.

### C4. `nil` vs empty slice (Go)
```go
var out []string
for _, u := range users { if u.Active { out = append(out, u.Name) } }
return out
```
"Simplifying" to `out := []string{}` or `make([]string, 0)` changes JSON from `null` to `[]` and `out == nil` checks. Keep it.

### C5. Compression that hurts reading (Python)
```python
# Rejected "simplification"
return {k: (v if not isinstance(v, dict) else f(v)) for k, v in d.items() if k not in skip and (v or k in keep)}
```
Fewer lines, more concepts per line. A loop with named conditions is the smaller *readable* equivalent.

### C6. Wrong abstraction (any)
Two validators look alike — `validate_invoice` and `validate_refund` both check amount > 0 and a currency code — but finance changes refund rules independently. Merging them into `validate_money_doc(kind)` adds a flag and couples them. Leave both; at most extract a shared `is_valid_currency` if the currency rule truly is one rule.

### C7. `Collectors.toList()` → `toList()` (Java)
```java
List<String> names = users.stream().map(User::name).collect(Collectors.toList());
names.add("system");
```
`Stream.toList()` returns an unmodifiable list — `add` throws. Keep `collect(Collectors.toList())` or `new ArrayList<>(…)`.

### C8. `NOT EXISTS` → `NOT IN` (SQL)
```sql
SELECT * FROM users u WHERE NOT EXISTS (SELECT 1 FROM bans b WHERE b.user_id = u.id);
```
`WHERE u.id NOT IN (SELECT user_id FROM bans)` looks shorter, but returns *no rows* if any `bans.user_id` is NULL. Keep `NOT EXISTS`.

### C9. Already minimal (Go)
```go
func readName(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    if len(data) == 0 {
        return nil, nil
    }
    return data, nil
}
```
Each branch is a distinct contract (error, empty → `nil`, data). No helper, wrapping, or merge reduces concepts. Correct response: "Already minimal; no change", optionally noting that `nil, nil` for empty files is a design choice callers depend on.

### C10. `put` loop → `Collectors.toMap` (Java)
```java
// Original
Map<String, User> byEmail = new HashMap<>();
for (User u : users) {
    byEmail.put(u.getEmail(), u);
}
// Tempting
Map<String, User> byEmail = users.stream().collect(Collectors.toMap(User::getEmail, u -> u));
```
Duplicate emails: the loop keeps the last user, `toMap` throws `IllegalStateException`. A null map value would throw `NullPointerException` (not here since `u` is non-null, but `toMap(User::getEmail, User::getNickname)` with a null nickname would). Safe only as `toMap(User::getEmail, u -> u, (a, b) -> b, HashMap::new)` *and* if values are never null — at which point the loop is no longer more verbose. Prefer keeping the loop.

### C11. `for..in` and default `sort()` (TS)
```ts
// Original
export function sortedKeys(obj: any) {
  var keys = [];
  for (var k in obj) keys.push(k);
  return keys.sort();
}
// Safe cleanup
export function sortedKeys(obj: object): string[] {
  const keys: string[] = [];
  for (const k in obj) keys.push(k);
  return keys.sort();
}
```
Rejected: `Object.keys(obj).sort((a, b) => a.localeCompare(b))`. `Object.keys` drops inherited enumerable keys (`Object.create({ inherited: 1 })`), and `localeCompare`/numeric comparators change the order the default string sort produced (`['10', '9']` stays `['10', '9']`). `obj: object` instead of `any` keeps callers compiling because every non-primitive argument still fits — confirm callers don't pass primitives (`sortedKeys('ab')` compiled before, not now).

### C12. "Make it strict everywhere" (PHP)
```php
// Original
function canEdit($role, $flags) {
    if (empty($role)) return false;
    return in_array($role, ['1', 'admin']) && $flags['edit'] == true;
}
```
User asks for `===`, strict `in_array`, and no `empty()`. Each is a behavior change:
- `in_array(1, ['1', 'admin'])` is true loose, false strict (int role ID from the DB).
- `$flags['edit'] == true` accepts `1`, `'yes'`, `'on'`; `=== true` only `true`.
- `empty($role)` rejects `'0'` and suppresses notices; `$role === ''` doesn't.

Apply only where types are guaranteed identical; for the rest, report each with the input that changes. Safe part: nothing here unless `$role` is typed `string` and `$flags['edit']` is `bool`.

### C13. Sequential `await` → `Promise.all` (TS)
```ts
// Original
export async function runAll(migrations: Migration[]) {
  for (const m of migrations) {
    await m.up();
  }
}
// Tempting
await Promise.all(migrations.map(m => m.up()));
```
Migrations then run concurrently (order not guaranteed) and continue after one fails. That's a performance/behavior change, not a simplification — keep the loop and say so.

### C14. Removing `any` without re-running the type checker (TS)
```ts
// Original
export function groupBy(items: any[], key: string): any {
  const out: any = {};
  for (const item of items) (out[item[key]] ||= []).push(item);
  return out;
}
// Looks right, fails `tsc --strict`
export function groupBy<T, K extends keyof T>(items: T[], key: K): Record<string, T[]> {
  const out: Record<string, T[]> = {};
  for (const item of items) (out[item[key]] ||= []).push(item); // TS2536: Type 'T[K]' cannot be used to index type 'Record<string, T[]>'
  return out;
}
```
Fix with `const k = String(item[key])` (matches runtime key coercion for non-symbol values) and then re-run `tsc`. Never report "verified with tsc" unless the command ran and passed — write the actual command and result, or "not run".

---

## D. Report samples

**Changed code:**
```
Summary: simplified `processOrder` (orders.py); behavior and API unchanged.
Changes:
- Guard clauses: removed 3 nesting levels (invalid order, empty cart).
- Redundancy: inlined 2 single-use private helpers; merged duplicate tax branches.
- Dead code: removed unused import `json` and a commented-out legacy block.
- Names/constants: FREE_SHIPPING_MIN, TAX_RATE.
Not changed: `x or default` in `apply_discount` — 0 is a valid discount, `??`-style change would differ.
Needs approval: `legacy_export()` duplicates `export()` but is public; propose a thin wrapper.
Verification: pytest 42 passed (`pytest tests/test_orders.py`); `ruff check orders.py` clean; checked empty cart, 0 discount, None customer.
```

**No change:**
```
Summary: `readName` is already minimal; no edits.
Not changed: considered merging the empty-file branch, but `nil, nil` vs `[]byte{}` is observable by callers.
Verification: not run (no repository); reasoned through error, empty-file, and data cases.
```

**Partly verified:**
```
Summary: typed `groupBy`/`sortedKeys` (collections.ts); runtime behavior unchanged.
Not changed: kept `for..in` (inherited keys) and default `sort()` (string order).
Verification: `npx tsc --strict --noEmit src/util/collections.ts` passed; unit tests not run (no test script in package.json).
```
