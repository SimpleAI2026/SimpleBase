const $ = selector => document.querySelector(selector)
async function api(path, options) { const res = await fetch(path, { headers: { 'content-type': 'application/json' }, ...options }); const data = await res.json(); if (!res.ok) throw new Error(data.error || '请求失败'); return data }
async function load() {
  try {
    const [posts, trending] = await Promise.all([api('/api/posts'), api('/api/trending')])
    $('#posts').replaceChildren()
    if (!posts.length) $('#posts').textContent = '还没有动态，发表第一条吧。'
    for (const post of posts) {
      const row = document.createElement('article'); row.className = 'post'
      const date = document.createElement('small'); date.textContent = post.created_at ? new Date(post.created_at).toLocaleString('zh-CN') : '刚刚'
      const text = document.createElement('p'); text.textContent = post.body
      const button = document.createElement('button'); button.textContent = '♡ 点赞'
      button.addEventListener('click', async () => { try { await api(`/api/posts/${encodeURIComponent(post.id)}/likes`, { method: 'POST', body: '{}' }); $('#message').textContent = '已点赞' } catch (error) { $('#message').textContent = error.message } })
      row.append(date, text, button); $('#posts').append(row)
    }
    $('#trending').textContent = trending.length ? trending.map(item => `${item.post_id.slice(0, 8)} · 热度 ${item.score}`).join('　/　') : '热榜将在定时任务完成后显示。'
  } catch (error) { $('#message').textContent = error.message }
}
$('#form').addEventListener('submit', async event => { event.preventDefault(); try { await api('/api/posts', { method: 'POST', body: JSON.stringify({ body: $('#body').value }) }); $('#body').value = ''; $('#message').textContent = '动态已发布'; await load() } catch (error) { $('#message').textContent = error.message } })
load()
