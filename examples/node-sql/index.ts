import { createClient, SimpleBaseError } from '@simplebase/sdk'

const url = process.env.SIMPLEBASE_URL || 'http://127.0.0.1:8080'
const apiKey = process.env.SIMPLEBASE_API_KEY || 'sb_live_dev_key_12345'
const projectId =
  process.env.SIMPLEBASE_PROJECT_ID || '00000000-0000-0000-0000-000000000002'
const databaseId = process.env.SIMPLEBASE_DATABASE_ID

async function main() {
  const sb = createClient({ url, apiKey, projectId, databaseId })
  const dbId = databaseId || (await sb.databases.list()).databases?.[0]?.id
  if (!dbId) throw new Error('no database — set SIMPLEBASE_DATABASE_ID or create one')

  const db = sb.database(dbId)
  await db.sql.execute(
    'CREATE TABLE IF NOT EXISTS sdk_demo (id VARCHAR PRIMARY KEY, note VARCHAR, created_at TIMESTAMP)'
  )
  const id = `row-${Date.now()}`
  await db.sql.execute('INSERT INTO sdk_demo (id, note, created_at) VALUES (?, ?, ?)', [
    id,
    'hello from @simplebase/sdk',
    new Date().toISOString()
  ])
  const q = await db.sql.query('SELECT id, note, created_at FROM sdk_demo WHERE id = ?', [id])
  console.log('columns', q.columns)
  console.log('rows', q.rows)
}

main().catch((e) => {
  if (e instanceof SimpleBaseError) {
    console.error(`SimpleBaseError ${e.status} ${e.code}: ${e.message}`, e.requestId)
  } else {
    console.error(e)
  }
  process.exit(1)
})
