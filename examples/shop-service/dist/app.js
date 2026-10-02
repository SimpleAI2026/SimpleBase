const $ = (id) => document.getElementById(id)
let selected = null
async function request(path, options) {
  const response = await fetch(path, { headers: { 'content-type': 'application/json' }, ...options })
  const data = await response.json()
  if (!response.ok) throw new Error(data.error || '服务暂不可用')
  return data
}
const money = (amount) => `¥${(Number(amount) / 100).toFixed(2)}`
async function load() {
  try {
    const items = await request('/api/products')
    $('status').textContent = `${items.length} 件精选好物`
    $('products').replaceChildren()
    const art = { tea: '🍵', lamp: '✦', book: '▤' }
    for (const item of items) {
      const card = document.createElement('article')
      card.className = 'card'
      const illustration = document.createElement('div')
      illustration.className = 'art'
      illustration.textContent = art[item.id] || '◇'
      const detail = document.createElement('div')
      detail.className = 'details'
      const heading = document.createElement('h3')
      heading.textContent = item.name
      const stock = document.createElement('p')
      stock.textContent = `现货 ${item.stock} 件 · 精选日常`
      const bottom = document.createElement('div')
      bottom.className = 'card-bottom'
      const price = document.createElement('span')
      price.className = 'price'
      price.textContent = money(item.price_minor)
      const button = document.createElement('button')
      button.textContent = '加入购物袋 +'
      button.disabled = Number(item.stock) < 1
      button.addEventListener('click', async () => {
        try {
          selected = { productId: item.id, qty: 1 }
          await request('/api/cart', { method: 'PUT', body: JSON.stringify(selected) })
          $('cart').textContent = `${item.name} × 1 · ${money(item.price_minor)}`
          $('buy').disabled = false
          $('result').textContent = '已加入购物袋'
        } catch (error) { $('result').textContent = error.message }
      })
      bottom.append(price, button)
      detail.append(heading, stock, bottom)
      card.append(illustration, detail)
      $('products').append(card)
    }
  } catch (error) { $('status').textContent = error.message }
}
$('buy').addEventListener('click', async () => {
  if (!selected) return
  $('buy').disabled = true
  try {
    const order = await request('/api/orders', { method: 'POST', body: JSON.stringify(selected) })
    $('result').textContent = `订单已创建 · 编号 ${order.id} · ${money(order.totalMinor)}`
    selected = null
    $('cart').textContent = '期待下次再见。'
    await load()
  } catch (error) { $('result').textContent = error.message; $('buy').disabled = false }
})
load()
