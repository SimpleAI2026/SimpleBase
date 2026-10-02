const cards = document.querySelector('#cards'), status = document.querySelector('#status'), message = document.querySelector('#message')
async function api(path, options) { const response = await fetch(path, { headers: { 'content-type': 'application/json' }, ...options }); const data = await response.json(); if (!response.ok) throw new Error(data.error || '请求失败'); return data }
async function load() {
  try {
    const slots = await api('/api/slots')
    status.textContent = `${slots.length} 个开放时段`
    cards.replaceChildren()
    for (const slot of slots) {
      const card = document.createElement('article'); card.className = 'card'
      const caption = document.createElement('p'); caption.textContent = 'PRIVATE BOOKING / 私享时段'
      const time = document.createElement('time'); time.textContent = new Date(slot.starts_at).toLocaleString('zh-CN')
      const remaining = document.createElement('p'); remaining.textContent = `容量 ${slot.capacity} 人 · 预约保留 2 分钟`
      const button = document.createElement('button'); button.textContent = '预约这个时段 ↗'
      button.addEventListener('click', async () => {
        button.disabled = true
        try {
          const hold = await api('/api/holds', { method: 'POST', body: JSON.stringify({ slotId: slot.id }) })
          const booking = await api('/api/bookings', { method: 'POST', body: JSON.stringify({ token: hold.token }) })
          message.textContent = `预约成功 · ${booking.id}`
        } catch (error) { message.textContent = error.message; button.disabled = false }
      })
      card.append(caption, time, remaining, button); cards.append(card)
    }
  } catch (error) { status.textContent = error.message }
}
load()
