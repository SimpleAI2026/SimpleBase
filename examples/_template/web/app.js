const $ = (id) => document.getElementById(id)
const state = { loading: false }

async function api(path, options) {
  const response = await fetch(path, { headers: { 'content-type': 'application/json' }, ...options })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(payload.error || `HTTP ${response.status}`)
  return payload
}

function tip(message, kind) {
  const node = $('form-tip')
  node.textContent = message
  node.className = `tip${kind ? ` ${kind}` : ''}`
}

function render(data) {
  $('views').textContent = data.views
  const list = $('item-list')
  list.innerHTML = ''
  $('empty-tip').style.display = data.items.length ? 'none' : ''
  for (const item of data.items) {
    const li = document.createElement('li')
    const title = document.createElement('span')
    title.textContent = item.title
    const time = document.createElement('span')
    time.className = 'time'
    time.textContent = new Date(item.created_at).toLocaleString()
    li.append(title, time)
    list.append(li)
  }
}

function renderStats(rows) {
  const list = $('stats-list')
  list.innerHTML = ''
  $('stats-empty').style.display = rows.length ? 'none' : ''
  for (const row of rows) {
    const li = document.createElement('li')
    const window = document.createElement('span')
    window.textContent = row.window_start
    const count = document.createElement('span')
    count.textContent = `${row.count} 条`
    li.append(window, count)
    list.append(li)
  }
}

async function refresh() {
  render(await api('/api/items'))
  renderStats(await api('/api/stats'))
}

$('item-form').addEventListener('submit', async (event) => {
  event.preventDefault()
  if (state.loading) return
  const input = $('title')
  const button = event.target.querySelector('button')
  state.loading = true
  button.disabled = true
  try {
    await api('/api/items', { method: 'POST', body: JSON.stringify({ title: input.value }) })
    input.value = ''
    tip('已添加', 'ok')
    await refresh()
  } catch (error) {
    tip(error.message, 'error')
  } finally {
    state.loading = false
    button.disabled = false
  }
})

refresh().catch((error) => tip(error.message, 'error'))
setInterval(() => refresh().catch(() => {}), 15000)
