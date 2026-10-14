// usage: node test.js <impl.js>
const assert = require('assert');
const { loadReport } = require(require('path').resolve(process.argv[2]));
function fakeDb(failAt, delays = { u: 5, o: 1, i: 10 }) {
  const logs = [];
  const call = (name, d, val) => (id, cb) => setTimeout(() => {
    logs.push('call:' + name);
    failAt === name ? cb(new Error(name + ' failed')) : cb(null, val);
  }, d);
  return { logs, log: m => logs.push(m),
    getUser: call('user', delays.u, { id: 7, name: 'Ann' }),
    getOrders: call('orders', delays.o, [{ amount: 3 }, { amount: 4.5 }]),
    getInvoice: call('invoice', delays.i, { total: 7.5 }) };
}
const run = db => new Promise(r => { const ret = loadReport(db, 7, (e, v) => r({ e, v, ret })); });
(async () => {
  let db = fakeDb(null); let { e, v } = await run(db);
  assert.strictEqual(e, null);
  assert.deepStrictEqual(v, { name: 'Ann', orders: 2, total: 7.5, invoiceTotal: 7.5 });
  assert.deepStrictEqual(db.logs, ['call:user', 'user Ann', 'call:orders', 'orders 2', 'call:invoice', 'invoice 7.5']);
  for (const f of ['user', 'orders', 'invoice']) {
    db = fakeDb(f); ({ e, v } = await run(db));
    assert.ok(e instanceof Error && e.message === f + ' failed', 'error passthrough ' + f);
    assert.strictEqual(v, undefined);
    assert.ok(!db.logs.includes('call:' + (f === 'user' ? 'orders' : f === 'orders' ? 'invoice' : 'none')), 'stops after ' + f);
  }
  console.log('PASS');
})().catch(err => { console.log('FAIL', err.message); process.exit(1); });
