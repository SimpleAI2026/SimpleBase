import { createClient, SimpleBaseError } from '@simplebase/sdk'

const url = process.env.SIMPLEBASE_URL || 'http://127.0.0.1:8080'
const apiKey = process.env.SIMPLEBASE_API_KEY || 'sb_live_dev_key_12345'
const projectId =
  process.env.SIMPLEBASE_PROJECT_ID || '00000000-0000-0000-0000-000000000002'
const databaseId = process.env.SIMPLEBASE_DATABASE_ID

async function main() {
  const sb = createClient({ url, apiKey, projectId })
  const dbId = databaseId || (await sb.databases.list()).databases?.[0]?.id
  if (!dbId) throw new Error('set SIMPLEBASE_DATABASE_ID')

  const db = sb.database(dbId)
  const name = 'sdk_notes'
  const listed = await db.collections.list()
  if (!listed.collections?.includes(name)) {
    await db.collections.create(name)
  }
  const col = db.collection(name)
  const doc = await col.insert({ title: 'from sdk', body: 'document kv demo' })
  console.log('inserted', doc)
  const all = await col.list()
  console.log('count', all.rows?.length)
  if (doc && typeof doc === 'object' && 'id' in doc && doc.id) {
    await col.update(String(doc.id), { title: 'updated', body: 'ok' })
    await col.remove(String(doc.id))
    console.log('updated + removed', doc.id)
  }
}

main().catch((e) => {
  if (e instanceof SimpleBaseError) console.error(e.status, e.code, e.message)
  else console.error(e)
  process.exit(1)
})
