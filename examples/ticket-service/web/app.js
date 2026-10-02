const $ = selector => document.querySelector(selector)
async function api(path, options) { const response = await fetch(path, { headers: { 'content-type': 'application/json' }, ...options }); const data = await response.json(); if (!response.ok) throw new Error(data.error || '请求失败'); return data }
async function load() {
  try {
    const [tickets, overdue] = await Promise.all([api('/api/tickets'), api('/api/overdue')])
    $('#tickets').replaceChildren()
    if (!tickets.length) $('#tickets').textContent = '暂时没有工单，创建第一张吧。'
    for (const ticket of tickets) {
      const article = document.createElement('article')
      const detail = document.createElement('div'), title = document.createElement('strong'), caption = document.createElement('small')
      title.textContent = ticket.title; caption.textContent = `${ticket.status === 'open' ? '处理中' : '已完成'} · 截止 ${new Date(ticket.due_at).toLocaleString('zh-CN')}`
      detail.append(title, caption); article.append(detail)
      if (ticket.status === 'open') {
        const button = document.createElement('button'); button.textContent = '标记完成'
        button.addEventListener('click', async () => {
          try { await api(`/api/tickets/${ticket.id}`, { method: 'PATCH', body: JSON.stringify({ status: 'closed', version: Number(ticket.version) }) }); $('#status').textContent = '工单已完成'; await load() } catch (error) { $('#status').textContent = error.message }
        })
        article.append(button)
      }
      $('#tickets').append(article)
    }
    $('#alerts').textContent = overdue.length ? `${overdue.length} 条超期提醒 · 请尽快处理` : '所有工单都在计划之中。'
  } catch (error) { $('#status').textContent = error.message }
}
$('#form').addEventListener('submit', async event => {
  event.preventDefault()
  try { await api('/api/tickets', { method: 'POST', body: JSON.stringify({ title: $('#title').value, priority: Number($('#priority').value) }) }); $('#title').value = ''; $('#status').textContent = '工单已创建'; await load() } catch (error) { $('#status').textContent = error.message }
})
load()
