import { cp, mkdir, rm, stat } from 'node:fs/promises'

const name = process.argv[2]
if (!/^[a-z][a-z0-9-]{0,38}$/.test(name)) throw new Error('Specify a case name (kebab-case)')
const root = new URL(`../../${name}/`, import.meta.url)
try { await stat(new URL('web/index.html', root)) } catch { throw new Error(`examples/${name}/web/index.html not found`) }
await rm(new URL('dist/', root), { recursive: true, force: true })
await mkdir(new URL('dist/', root), { recursive: true })
for (const file of ['index.html', 'style.css', 'app.js']) await cp(new URL(`web/${file}`, root), new URL(`dist/${file}`, root))
console.log(`Built ${name}/dist`)
