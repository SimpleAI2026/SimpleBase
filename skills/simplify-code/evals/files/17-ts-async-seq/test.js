const { execSync } = require('child_process'); const fs = require('fs'); const path = require('path'); const os = require('os'); const assert = require('assert');
const d = fs.mkdtempSync(path.join(os.tmpdir(), 'ts17-')); fs.copyFileSync(path.resolve(process.argv[2]), path.join(d, 'm.ts'));
try { execSync(`tsc --strict --target es2022 --module commonjs --lib es2022 --outDir ${d}/out ${d}/m.ts`, { stdio: 'pipe' }); }
catch (e) { console.log('FAIL tsc', (e.stdout || '').toString().slice(0, 800)); process.exit(1); }
const { runAll } = require(path.join(d, 'out', 'm.js'));
(async () => {
  try {
    const ev = []; const log = (s) => ev.push(s);
    const mk = (n, ms, fail) => ({ name: n, run: () => new Promise((res, rej) => { ev.push('run ' + n); setTimeout(() => { ev.push('end ' + n); fail ? rej(new Error(n)) : res(n); }, ms); }) });
    const r = await runAll([mk('a', 30), mk('b', 5, true), mk('c', 1)], log);
    await new Promise((r) => setTimeout(r, 60));
    assert.deepStrictEqual(r, ['a']);
    assert.deepStrictEqual(ev, ['start a', 'run a', 'end a', 'start b', 'run b', 'end b', 'fail b'], 'sequential, stops at first failure, c never started: ' + JSON.stringify(ev));
    const ev2 = []; assert.deepStrictEqual(await runAll([], (s) => ev2.push(s)), []);
    const throwsSync = { name: 'x', run: () => { throw new Error('sync'); } };
    assert.deepStrictEqual(await runAll([throwsSync], () => {}), []); // sync throw is caught
    console.log('PASS');
  } catch (e) { console.log('FAIL', e.message); process.exit(1); }
})();
