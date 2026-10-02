import { cp, mkdir, rm } from 'node:fs/promises'
const root = new URL('../', import.meta.url)
await rm(new URL('dist/', root), { recursive: true, force: true })
await mkdir(new URL('dist/', root), { recursive: true })
for (const file of ['index.html', 'style.css', 'app.js']) await cp(new URL(`web/${file}`, root), new URL(`dist/${file}`, root))
console.log('Built shop-service/dist (static assets; API requires trusted BFF)')
