// differential: impl vs original on negative / fractional / NaN page & size
const assert = require('assert'), path = require('path');
const neu = require(path.resolve(process.argv[2])).paginate, old = require(path.join(__dirname, 'input.js')).paginate;
const xs = Array.from({ length: 45 }, (_, i) => i); const diffs = [];
const vals = [-2, -1, 0.5, 1.5, 3, NaN, -5, 2.5, 100];
for (const page of vals) for (const size of [undefined, 10, -5, 2.5, NaN]) for (const sort of [undefined, 'desc']) {
  const o = { page, size, sort };
  try { assert.deepStrictEqual(neu(xs, { ...o }), old(xs, { ...o })); } catch (e) { diffs.push(JSON.stringify(o)); }
}
console.log(diffs.length ? `FAIL ${diffs.length} diffs e.g. ${diffs.slice(0, 3).join(' ')}` : 'PASS'); process.exit(diffs.length ? 1 : 0);
