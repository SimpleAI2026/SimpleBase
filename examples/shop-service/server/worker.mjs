export function createWorker({ sb, sql, projectId, onError = console.error }) {
  const path = (suffix) => `/v1/projects/${encodeURIComponent(projectId)}${suffix}`
  const rows = (value) => value.rows.map(row => Object.fromEntries(value.columns.map((key, i) => [key, row[i]])))
  let lastSeen = null
  async function poll() {
    const jobs = await sb.raw.request('GET', path('/cron-jobs'))
    const job = jobs.jobs?.find(entry => entry.name === 'ex_shop_daily')
    if (!job) return
    const { runs } = await sb.raw.request('GET', path(`/cron-jobs/${job.id}/runs?limit=100`))
    if (!Array.isArray(runs)) throw new Error('Invalid cron runs response')
    if (runs.length === 100 && (!lastSeen || !runs.some(run => run.id === lastSeen))) throw new Error('Cron run history exceeded 100 records; stop worker and backfill manually')
    for (const run of [...runs].reverse()) {
      if (run.status !== 'completed') continue
      const exists = rows(await sql.query('SELECT run_id FROM ex_shop_processed_runs WHERE run_id = ?', [run.id], { maxRows: 1 }))
      if (exists.length) continue
      const window = JSON.parse(run.response_json || '')
      const start = Date.parse(window.window_start)
      const end = Date.parse(window.window_end)
      if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start || end - start > 86400000 || end > Date.now() + 60000) throw new Error('Invalid cron window')
      const sales = rows(await sql.query('SELECT i.qty, i.price_minor FROM ex_shop_order_items i JOIN ex_shop_orders o ON o.id = i.order_id WHERE o.created_at >= ? AND o.created_at < ? LIMIT 500', [window.window_start, window.window_end], { maxRows: 500 }))
      if (sales.length === 500) throw new Error('Too many sales; backfill window with pagination before continuing')
      const input = { rows: sales.map(entry => ({ qty: Number(entry.qty), amount_minor: Number(entry.qty) * Number(entry.price_minor) })) }
      if (JSON.stringify(input).length > 30000) throw new Error('Function input exceeds conservative demo limit')
      const summary = await sb.raw.request('POST', `/go/${encodeURIComponent(projectId)}/ex_shop_metrics/Compute`, { body: input })
      if (!Number.isSafeInteger(summary.units) || !Number.isSafeInteger(summary.amount_minor)) throw new Error('Invalid summary result')
      const result = await sql.batch([
        { sql: 'INSERT INTO ex_shop_daily_stats SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM ex_shop_daily_stats WHERE window_start = ?)', args: [window.window_start, summary.units, summary.amount_minor, run.id, window.window_start] },
        { sql: 'INSERT INTO ex_shop_processed_runs (run_id, window_start) VALUES (?, ?)', args: [run.id, window.window_start] }
      ], { transactional: true })
      if (result.error || result.results?.some(item => item.error_code)) throw new Error('Failed to persist cron run')
      try { await sb.raw.request('POST', path('/kv'), { body: { type: 'cmd', argvs: ['SET', 'ex:shop:popular', JSON.stringify(summary), 'EX', '3600'] } }) } catch (error) { onError(`Shop cache refresh failed: ${error.message}`) }
    }
    if (runs.length) lastSeen = runs[0].id
  }
  return { poll }
}

export function startWorker(opts) {
  const worker = createWorker(opts)
  let busy = false
  const timer = setInterval(async () => {
    if (busy) return
    busy = true
    try { await worker.poll() } catch (error) { opts.onError?.(`Shop worker stopped: ${error.message}`); clearInterval(timer) } finally { busy = false }
  }, 15000)
  timer.unref()
  return () => clearInterval(timer)
}
