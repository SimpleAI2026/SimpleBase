// usage: node test.js <impl.ts>  (compiles with tsc, then characterizes)
const { execSync } = require('child_process'); const fs = require('fs'); const path = require('path'); const os = require('os');
const assert = require('assert');
const impl = path.resolve(process.argv[2]); const d = fs.mkdtempSync(path.join(os.tmpdir(), 'ts14-'));
fs.copyFileSync(impl, path.join(d, 'm.ts'));
try { execSync(`tsc --strict --target es2022 --module commonjs --lib es2022 --outDir ${d}/out ${d}/m.ts`, { stdio: 'pipe' }); }
catch (e) { console.log('FAIL tsc', (e.stdout || '').toString().slice(0, 800)); process.exit(1); }
const m = require(path.join(d, 'out', 'm.js'));
try {
  const g = m.groupBy([{ n: 1, c: 'a' }, { n: 2, c: 'b' }, { n: 3, c: 'a' }], (x) => x.c);
  assert.deepStrictEqual(JSON.parse(JSON.stringify(g)), { a: [{ n: 1, c: 'a' }, { n: 3, c: 'a' }], b: [{ n: 2, c: 'b' }] });
  assert.strictEqual(Object.getPrototypeOf(g), Object.prototype, 'must stay a plain object (not Map / null-proto)');
  const g2 = m.groupBy([5, 15, 25], (x) => x % 10); assert.deepStrictEqual(Object.keys(g2), ['5']);
  assert.deepStrictEqual(m.sortedKeys({ 10: 1, 9: 1, b: 1, a: 1 }), ['10', '9', 'a', 'b']); // lexicographic, not numeric
  const proto = { inherited: 1 }; const rec = Object.create(proto); rec.own = 1;
  assert.deepStrictEqual(m.sortedKeys(rec), ['inherited', 'own']); // for..in includes inherited enumerables
  console.log('PASS');
} catch (e) { console.log('FAIL', e.message); process.exit(1); }
