const assert = require('assert'); const { paginate } = require(require('path').resolve(process.argv[2]));
const xs = Array.from({ length: 45 }, (_, i) => i);
try {
  assert.deepStrictEqual(paginate(xs, {}).items, xs.slice(0, 20));
  let r = paginate(xs, { page: 0, size: 0 }); assert.strictEqual(r.page, 1); assert.strictEqual(r.size, 20); // 0 -> defaults
  r = paginate(xs, { page: '', size: null }); assert.strictEqual(r.page, 1); assert.strictEqual(r.size, 20);
  r = paginate(xs, { page: '2', size: '10' }); assert.deepStrictEqual(r.items, xs.slice(10)); assert.strictEqual(r.page, '2'); // characterization: string concat quirk ((2-1)*'10' + '10' = '1010')
  r = paginate(xs, { page: 3, size: 20, sort: 'desc' }); assert.deepStrictEqual(r.items, [4, 3, 2, 1, 0]);
  r = paginate(xs, { sort: new String('desc') }); assert.strictEqual(r.items[0], 44); // loose == on String object
  const orig = xs.slice(); paginate(xs, { sort: 'desc' }); assert.deepStrictEqual(xs, orig);
  console.log('PASS');
} catch (e) { console.log('FAIL', e.message); process.exit(1); }
