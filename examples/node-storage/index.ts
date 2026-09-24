import { createClient, SimpleBaseError } from '@simplebase/sdk'

const url = process.env.SIMPLEBASE_URL || 'http://127.0.0.1:8080'
const apiKey = process.env.SIMPLEBASE_API_KEY || 'sb_live_dev_key_12345'
const projectId =
  process.env.SIMPLEBASE_PROJECT_ID || 'dev-shop'

async function main() {
  const sb = createClient({ url, apiKey, projectId })
  const key = `sdk-demo/hello-${Date.now()}.txt`
  const uploaded = await sb.storage.upload(key, 'hello storage from sdk\n', {
    contentType: 'text/plain',
    filename: 'hello.txt'
  })
  console.log('uploaded', uploaded)
  const list = await sb.storage.list({ prefix: 'sdk-demo/' })
  console.log(
    'list',
    list.map((o) => o.key)
  )
  const { url: signed } = await sb.storage.presign(key)
  console.log('presign', signed)
  await sb.storage.remove(key)
  console.log('removed', key)
}

main().catch((e) => {
  if (e instanceof SimpleBaseError) console.error(e.status, e.code, e.message)
  else console.error(e)
  process.exit(1)
})
