const $ = selector => document.querySelector(selector)
let sessionId = ''
async function api(path, options) { const res = await fetch(path, { headers: { 'content-type': 'application/json' }, ...options }); const data = await res.json(); if (!res.ok) throw new Error(data.error || '请求失败'); return data }
async function ranks() {
  try { const data = await api('/api/leaderboard'); $('#ranking').replaceChildren(); if (!data.length) $('#ranking').textContent = '排行榜尚无记录，快来成为第一名。'; data.forEach((item, index) => { const row = document.createElement('article'), label = document.createElement('span'), score = document.createElement('b'); label.textContent = `#${index + 1}  ${item.player_id}`; score.textContent = `${item.score} PTS`; row.append(label, score); $('#ranking').append(row) }) } catch (error) { $('#ranking').textContent = error.message }
}
$('#start').addEventListener('click', async () => {
  try { const [levels, session] = await Promise.all([api('/api/levels'), api('/api/sessions', { method: 'POST', body: '{}' })]); sessionId = session.sessionId; $('#question').textContent = levels[0]?.title || '回答问题'; $('#play').hidden = false; $('#message').textContent = '对局已开启，5 分钟内提交答案。' } catch (error) { $('#message').textContent = error.message }
})
$('#submit').addEventListener('click', async () => {
  try { const result = await api('/api/scores', { method: 'POST', body: JSON.stringify({ sessionId, playerId: $('#player').value, answer: $('#answer').value }) }); $('#message').textContent = `挑战完成！得分 ${result.score}`; $('#play').hidden = true; await ranks() } catch (error) { $('#message').textContent = error.message }
})
ranks()
